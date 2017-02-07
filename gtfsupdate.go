// AWS Lambda Service that updates stops-database every 12 hours
// by retrieving publicly-available GTFS data

package main

import (
	"github.com/geops/gtfsparser"
	_ "github.com/lib/pq"
	"fmt"
	"os"
	"log"
	"net/http"
	"io"
	"database/sql"
	"io/ioutil"
)

func getZip(url string, name string) {
	// Create output file
	newFile, err := os.Create(name)
	if err != nil {
		log.Fatal(err)
	}
	defer newFile.Close()

	// HTTP GET request devdungeon.com
	response, err := http.Get(url)
	defer response.Body.Close()

	// Write bytes from HTTP response to file.
	// response.Body satisfies the reader interface.
	// newFile satisfies the writer interface.
	// That allows us to use io.Copy which accepts
	// any type that implements reader and writer interface
	numBytesWritten, err := io.Copy(newFile, response.Body)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Downloaded %d byte file.\n", numBytesWritten)
}

func getdbpass() string {
	b, err := ioutil.ReadFile(".pass")
	if err != nil {
		fmt.Print(err)
	}
	return string(b) // convert content to a 'string'
}

func main() {

	feed := gtfsparser.NewFeed()
	grtLink := "http://www.regionofwaterloo.ca/opendatadownloads/GRT_GTFS.zip"
	grtName := "./src/grt.zip"
	getZip(grtLink, grtName)
	feed.Parse(grtName)

	fmt.Printf("Done, parsed %d agencies, %d stops, %d routes, %d trips, %d fare attributes\n\n",
	len(feed.Agencies), len(feed.Stops), len(feed.Routes), len(feed.Trips), len(feed.FareAttributes))

	for k, v := range feed.Stops {
		fmt.Printf("[%s] %s (@ %f,%f)\n", k, v.Name, v.Lat, v.Lon)
	}

	db, err := sql.Open("postgres", "postgres://eddy:" + getdbpass() +
		"@transistops.cchubqlyefe9.ca-central-1.rds.amazonaws.com/transistopsdb?sslmode=disable")
	 _, err1 := db.Exec("INSERT INTO stops (stop) VALUES (123)")
	if err != nil {
		log.Fatal(err)
	}
	if err1 != nil {
		log.Fatal(err1)
	}


	//rows, err := db.Query("SELECT name FROM users WHERE age = $1", age)
}