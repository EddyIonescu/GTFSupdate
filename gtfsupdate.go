/*

 Go app for AWS Lambda Service that updates stops-database every 12 hours
 by retrieving publicly-available GTFS data

 Libraries used: mgo mongodb driver, gtfs parser, object-to-hash

*/

package main

import (
	"github.com/geops/gtfsparser"
	"fmt"
	"os"
	"log"
	"net/http"
	"io"
	"io/ioutil"
	_ "gopkg.in/mgo.v2/bson"
	_ "container/list"
	"gopkg.in/mgo.v2"
	"strings"
	_ "github.com/go-mgo/mgo/bson"
	"github.com/mitchellh/hashstructure"
	"github.com/go-mgo/mgo/bson"
	"strconv"
)

func getZip(url string, name string) {

	newFile, err := os.Create(name)
	if err != nil {
		log.Fatal(err)
	}
	defer newFile.Close()

	response, err := http.Get(url)
	defer response.Body.Close()

	numBytesWritten, err := io.Copy(newFile, response.Body)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Downloaded %d byte file.\n", numBytesWritten)
}

func getdbpass() string {
	b, err := ioutil.ReadFile(".mongopass")
	if err != nil {
		fmt.Print(err)
	}
	return strings.TrimRight(string(b), "\n")
}

type location struct {
	Type string `json:"type"`
	Coordinates []float32 `json:"coordinates"`
}

type stop struct {
	Id bson.ObjectId `bson:"_id"`
	Name string `json:"name"`
	LocalCode string `json:"localcode"`
	LocalId string `json:"localid"`
	Location location `json:"location",bson:"location"`
	Agency agency `json:"agency"`
}

// contains info required to make realtime request
type realtime struct {
	NextBusAgencyId string `json:"nextbus_agency_id"`
	GtfsRTupdate string `json:"gtfsrt_update"`
	GtfsRTpositions string `json:"gtfsrt_positions"`
}

type agency struct {
	Name string `json:"name"`
	Link string `json:"gtfslink"`
	RealTime realtime `json:"realtime"`
}

func main() {
	links := []agency{
		{"waterloo-grt", "http://www.regionofwaterloo.ca/opendatadownloads/GRT_GTFS.zip",
			realtime{"", "http://192.237.29.212:8080/gtfsrealtime/TripUpdates",
				"http://192.237.29.212:8080/gtfsrealtime/VehiclePositions"}},
		{"toronto-ttc", "http://opendata.toronto.ca/TTC/routes/OpenData_TTC_Schedules.zip", realtime{"ttc", "", ""}},
		{"sf-muni", "http://archives.sfmta.com/transitdata/google_transit.zip", realtime{"sf-muni", "", ""}},
		//{"cali-bart", "https://transitfeeds.com/link?u=http://www.bart.gov/dev/schedules/google_transit.zip",
		//	realtime{"", "http://api.bart.gov/gtfsrt/tripupdate.aspx", ""}},
	}

	// get gtfs file
	feeds := make([]*gtfsparser.Feed, len(links))
	for i, agency := range links {
		feed := gtfsparser.NewFeed()
		getZip(agency.Link, "./src/gtfs.zip")
		feed.Parse("./src/gtfs.zip")

		fmt.Printf("Done with %s, parsed %d agencies, %d stops, %d routes, %d trips, %d fare attributes\n\n",
			agency.Name, len(feed.Agencies), len(feed.Stops), len(feed.Routes), len(feed.Trips), len(feed.FareAttributes))

		feeds[i] = feed
	}

	// connect to mongodb
	info := mgo.DialInfo{}
	info.Addrs = []string{"ec2-52-60-121-211.ca-central-1.compute.amazonaws.com"}
	info.Database = "transistops"
	info.Username = "eddy"
	info.Password = getdbpass()
	info.Mechanism = "SCRAM-SHA-1"
	session, err := mgo.DialWithInfo(&info)

	if err != nil {
		panic(err)
	}
	defer session.Close()

	collection := session.DB("transistops").C("stops")

	// TODO update instead of dropping
	collection.DropCollection()

	// so that we can do geojson 2d-sphere queries (in transibot)
	index := mgo.Index{
		Key: []string{"$2dsphere:location"},
		Bits: 26,
	}
	err = collection.EnsureIndex(index)
	if err != nil {
		panic(err)
	}

	for agencyIndex, feed := range feeds {
		// iterate through stops and update database
		stops := make([]stop, len(feed.Stops))
		i := 0
		for k, v := range feed.Stops {
			stops[i] = (stop{
				Name: v.Name,
				LocalId: k,
				LocalCode: v.Code,
				Location: location{"Point", []float32{v.Lon, v.Lat}},
			})
			hash, err := hashstructure.Hash(stops[i], nil)
			if err != nil {
				panic(err)
			}
			stops[i].Id = bson.ObjectId(strconv.FormatUint(hash, 10)[0:12])
			stops[i].Agency = links[agencyIndex]
			//fmt.Printf("[%s] %s (@ %f,%f)\n", k, v.Name, v.Lat, v.Lon)
			//fmt.Println(hash)
			//fmt.Println(stops[i].Id)
			//fmt.Println(stops[i].Name)
			i++
		}

		bulk := collection.Bulk()
		for _, stop := range stops {
			//fmt.Println(stop.Id)
			//fmt.Println(stop.Name)
			if (stop.Id != "") {
				bulk.Insert(stop)
			}
		}
		_, err1 := bulk.Run()
		if err1 != nil {
			panic(err1)
		}
	}
}