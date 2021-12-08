package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
)

var fr24idCookie = getCookie()

type flightPosition struct {
	fr24id    string
	longitude float64
	latitude  float64
	altitude  int
}

type FlightDetailsJSON struct {
	Aircraft struct {
		Model struct {
			Text string
		}
	}

	Airline struct {
		Name string
	}

	Airport struct {
		Origin struct {
			Name string
		}

		Destination struct {
			Name string
		}
	}
}

func main() {
	http.HandleFunc("/", handler)
	log.Println("Server started")
	log.Fatal(http.ListenAndServe(":2107", nil))
}

func getCookie() string {
	resp, err := http.Get("https://www.flightradar24.com")
	if err != nil {
		log.Fatal(err)
	}

	cookies := resp.Header.Values("set-cookie")
	id := strings.Split(cookies[0], ";")[0]

	log.Println("Cookie set to " + id)
	return id
}

func handler(w http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		log.Println("GET received from " + request.UserAgent())

		longitude, err := strconv.ParseFloat(request.URL.Query().Get("longitude"), 64)
		latitude, err := strconv.ParseFloat(request.URL.Query().Get("latitude"), 64)
		altitude, err := strconv.ParseFloat(request.URL.Query().Get("altitude"), 64)

		response := formatFlight(getClosestFlight(longitude, latitude, altitude).fr24id)
		if response == "   " {
			response = "No aircraft found nearby"
		}

		_, err = fmt.Fprint(w, response)
		if err != nil {
			log.Println(err)
		} else {
			log.Println(response)
		}
	default:
		http.Error(w, "Incorrect method", http.StatusMethodNotAllowed)
	}
}

func formatFlight(flight string) string {
	airline, aircraft, origin, destination := getFlightDetails(flight)

	if origin != "" {
		origin = "from " + origin
	}

	if destination != "" {
		destination = "to " + destination
	}

	return fmt.Sprintf("%s %s %s %s", airline, aircraft, origin, destination)
}

func getFlightDetails(fr24id string) (string, string, string, string) {
	url := "https://data-live.flightradar24.com/clickhandler/?flight=" + fr24id

	body := httpGet(url)
	details := parseFlightDetailsJSON(body)
	return details.Airline.Name, details.Aircraft.Model.Text, details.Airport.Origin.Name, details.Airport.Destination.Name
}

func parseFlightDetailsJSON(bytes []byte) FlightDetailsJSON {
	var response FlightDetailsJSON

	if err := json.Unmarshal(bytes, &response); err != nil {
		log.Println(err)
	}

	return response
}

func getClosestFlight(longitude float64, latitude float64, altitude float64) flightPosition {
	x, y, z := pointToCartesian(longitude, latitude, feetToMeters(altitude))

	minDistance := math.Inf(1)
	closestPlane := flightPosition{}

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

func getFlights(longitude float64, latitude float64) []flightPosition {
	const latitudeDelta = 0.5
	longitudeDelta := latitudeDelta * math.Cos(latitude)

	url := fmt.Sprintf("https://data-live.flightradar24.com/zones/fcgi/feed.js?faa=1&bounds=%f,%f,%f,%f"+
		"&satellite=1&mlat=1&flarm=1&adsb=1&gnd=0&air=1&vehicles=0&estimated=1&maxage=14400&gliders=0&stats=0",
		latitude+latitudeDelta, latitude-latitudeDelta, longitude-longitudeDelta, longitude+longitudeDelta)

	body := httpGet(url)
	return parseFlightsJSON(body)
}

func httpGet(url string) []byte {
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

	return body
}

func parseFlightsJSON(bytes []byte) []flightPosition {
	var response interface{}
	if err := json.Unmarshal(bytes, &response); err != nil {
		log.Println(err)
	}

	flightsData := response.(map[string]interface{})
	delete(flightsData, "full_count")
	delete(flightsData, "version")

	var flights []flightPosition
	for fr24id, planeData := range flightsData {
		status := planeData.([]interface{})
		var plane flightPosition
		plane.fr24id = fr24id
		plane.latitude = status[1].(float64)
		plane.longitude = status[2].(float64)
		plane.altitude = int(status[4].(float64))
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
