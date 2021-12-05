package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// data obtained from openflights.org
var icaoAirlines = mapRecords(loadCsv("data/airlines.csv"))
var icaoTypes = mapRecords(loadCsv("data/planes.csv"))
var iataAirports = mapRecords(loadCsv("data/airports.csv"))
var fr24idCookie = getCookie()

type flightData struct {
	icaoType      string
	icaoAirline   string
	iataDeparting string
	iataArriving  string
	longitude     float64
	latitude      float64
	altitude      int
}

func main() {
	http.HandleFunc("/", handler)
	log.Println("Server started")
	log.Fatal(http.ListenAndServe(":2107", nil))
}

func getCookie() string {
	resp, err := http.Get("https://www.flightradar24.com")
	if err != nil {
		log.Fatal("Couldn't fetch cookie")
	}

	cookies := resp.Header.Values("set-cookie")

	return strings.Split(cookies[0], ";")[0]
}

func handler(w http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		log.Println("GET received")

		longitude, err := strconv.ParseFloat(request.URL.Query().Get("longitude"), 64)
		latitude, err := strconv.ParseFloat(request.URL.Query().Get("latitude"), 64)
		altitude, err := strconv.ParseFloat(request.URL.Query().Get("altitude"), 64)

		response := formatFlight(getClosestPlane(longitude, latitude, altitude))

		_, err = fmt.Fprint(w, response)
		if err != nil {
			log.Println(err)
		}
	default:
		http.Error(w, "Incorrect method", http.StatusMethodNotAllowed)
	}
}

func formatFlight(flight flightData) string {
	airline := icaoAirlines[flight.icaoAirline]
	aircraft := icaoTypes[flight.icaoType]
	departing := iataAirports[flight.iataDeparting]
	arriving := iataAirports[flight.iataArriving]

	return fmt.Sprintf("%s %s from %s to %s", airline, aircraft, departing, arriving)
}

func getClosestPlane(longitude float64, latitude float64, altitude float64) flightData {
	x, y, z := pointToCartesian(longitude, latitude, feetToMeters(altitude))

	minDistance := math.Inf(1)
	closestPlane := flightData{}

	flights := getFlights(longitude, latitude)

	for _, flight := range flights {
		xFlight, yFlight, zFlight := pointToCartesian(flight.longitude, flight.latitude, float64(flight.altitude))
		distanceToFlight := distance(x, y, z, xFlight, yFlight, zFlight)
		if distanceToFlight < minDistance {
			minDistance = distanceToFlight
			closestPlane = flight
		}
	}

	return closestPlane
}

func getFlights(longitude float64, latitude float64) []flightData {
	const latitudeDelta = 0.2
	longitudeDelta := latitudeDelta * math.Cos(latitude)

	url := fmt.Sprintf("https://data-live.flightradar24.com/zones/fcgi/feed.js?faa=1&bounds=%f,%f,%f,%f"+
		"&satellite=1&mlat=1&flarm=1&adsb=1&gnd=0&air=1&vehicles=0&estimated=1&maxage=14400&gliders=0&stats=0",
		latitude+latitudeDelta, latitude-latitudeDelta, longitude-longitudeDelta, longitude+longitudeDelta)

	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Cookie", fr24idCookie)

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	return parseFlightsJSON(body)
}

func parseFlightsJSON(bytes []byte) []flightData {
	var response interface{}
	if err := json.Unmarshal(bytes, &response); err != nil {
		log.Println(err)
	}

	flightsData := response.(map[string]interface{})
	delete(flightsData, "full_count")
	delete(flightsData, "version")

	var flights []flightData
	for _, planeData := range flightsData {
		status := planeData.([]interface{})
		var plane flightData
		plane.latitude = status[1].(float64)
		plane.longitude = status[2].(float64)
		plane.altitude = int(status[4].(float64))
		plane.icaoType = status[8].(string)
		plane.iataDeparting = status[11].(string)
		plane.iataArriving = status[12].(string)
		plane.icaoAirline = status[18].(string)
		flights = append(flights, plane)
	}

	return flights
}

func feetToMeters(feet float64) float64 {
	return 0.3048 * feet
}

func distance(x1 float64, y1 float64, z1 float64, x2 float64, y2 float64, z2 float64) float64 {
	return math.Sqrt(math.Pow(x2-x1, 2) + math.Pow(y2-y1, 2) + math.Pow(z2-z1, 2))
}

func pointToCartesian(longitude float64, latitude float64, altitude float64) (float64, float64, float64) {
	// https://gis.stackexchange.com/a/278753

	N := func(phi float64) float64 {
		return 6378137 / math.Sqrt(1-0.006694379990197619*math.Pow(math.Sin(phi), 2))
	}

	x := (N(latitude) + altitude) * math.Cos(latitude) * math.Cos(longitude)
	y := (N(latitude) + altitude) * math.Cos(latitude) * math.Sin(longitude)
	z := (0.9933056200098024*N(latitude) + altitude) * math.Sin(latitude)

	return x, y, z
}

func loadCsv(path string) [][]string {
	log.Println("Loading " + path)

	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	csvReader := csv.NewReader(file)
	records, err := csvReader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	return records
}

func mapRecords(records [][]string) map[string]string {
	hashTable := make(map[string]string)

	for _, line := range records {
		hashTable[line[0]] = line[1]
	}

	return hashTable
}
