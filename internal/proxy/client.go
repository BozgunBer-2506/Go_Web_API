package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	rapidAPIKey = "8c145b23a1msh5288f08d5058e73p18692ejsn1e8fb15260c9"

	skyHost     = "skyscanner-flights-travel-api.p.rapidapi.com"
	skyBase     = "https://skyscanner-flights-travel-api.p.rapidapi.com"

	gfHost      = "google-flights2.p.rapidapi.com"
	gfBase      = "https://google-flights2.p.rapidapi.com"

	hotelsHost  = "hotels-com-provider.p.rapidapi.com"
	hotelsBase  = "https://hotels-com-provider.p.rapidapi.com"
)

// HotelData is the flat struct passed to templates and JSON API.
type HotelData struct {
	HotelID    int     `json:"hotel_id"`
	HotelName  string  `json:"hotel_name"`
	Price      float64 `json:"price"`
	Currency   string  `json:"currency"`
	Rating     float64 `json:"rating"`
	RatingWord string  `json:"rating_word"`
	PhotoURL   string  `json:"photo_url"`
	Stars      int     `json:"stars"`
}

// FlightData is the flat struct for flight offers.
type FlightData struct {
	FromCity        string  `json:"from_city"`
	ToCity          string  `json:"to_city"`
	FromCode        string  `json:"from_code"`
	ToCode          string  `json:"to_code"`
	DepartTime      string  `json:"depart_time"`
	ArriveTime      string  `json:"arrive_time"`
	DurationHours   int     `json:"duration_hours"`
	DurationMinutes int     `json:"duration_minutes"`
	Airline         string  `json:"airline"`
	AirlineLogo     string  `json:"airline_logo"`
	Price           float64 `json:"price"`
	Currency        string  `json:"currency"`
	Stops           int     `json:"stops"`
}

// DestSuggestion is returned by the /suggest endpoint (hotel city search).
type DestSuggestion struct {
	EntityID  string `json:"entityId"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Hierarchy string `json:"hierarchy"`
}

// FlightDestSuggestion is returned by the /suggest-flight endpoint.
type FlightDestSuggestion struct {
	SkyID       string `json:"skyId"`
	EntityID    string `json:"entityId"`
	Name        string `json:"name"`
	CityName    string `json:"cityName"`
	CountryName string `json:"countryName"`
	PlaceType   string `json:"placeType"`
}

// --- Google Flights internal structs ---

type gfAirportEntry struct {
	ID   string           `json:"id"`
	Type string           `json:"type"`
	Name string           `json:"title"`
	City string           `json:"city"`
	List []gfAirportEntry `json:"list"`
}

type gfAirportResponse struct {
	Data []gfAirportEntry `json:"data"`
}

type gfAirport struct {
	AirportCode string `json:"airport_code"`
	AirportName string `json:"airport_name"`
}

type gfFlightSegment struct {
	DepartureAirport gfAirport `json:"departure_airport"`
	ArrivalAirport   gfAirport `json:"arrival_airport"`
	Airline          string    `json:"airline"`
	AirlineLogo      string    `json:"airline_logo"`
}

type gfDuration struct {
	Raw int `json:"raw"`
}

type gfLayover struct {
	AirportCode string `json:"airport_code"`
}

type gfItinerary struct {
	DepartureTime string            `json:"departure_time"`
	ArrivalTime   string            `json:"arrival_time"`
	Duration      gfDuration        `json:"duration"`
	Flights       []gfFlightSegment `json:"flights"`
	Layovers      []gfLayover       `json:"layovers"`
	AirlineLogo   string            `json:"airline_logo"`
	Price         float64           `json:"price"`
}

type gfItineraries struct {
	TopFlights   []gfItinerary `json:"topFlights"`
	OtherFlights []gfItinerary `json:"otherFlights"`
}

type gfFlightData struct {
	Itineraries gfItineraries `json:"itineraries"`
}

type gfFlightResponse struct {
	Status bool         `json:"status"`
	Data   gfFlightData `json:"data"`
}

// --- Hotels.com regions internal structs ---

type hcRegion struct {
	GaiaID      string `json:"gaiaId"`
	Type        string `json:"type"`
	RegionNames struct {
		FullName    string `json:"fullName"`
		ShortName   string `json:"shortName"`
		DisplayName string `json:"displayName"`
	} `json:"regionNames"`
	HierarchyInfo struct {
		Country struct {
			Name string `json:"name"`
		} `json:"country"`
	} `json:"hierarchyInfo"`
}

type hcRegionResponse struct {
	Data []hcRegion `json:"data"`
}

// ---

type ProxyClient struct {
	HTTPClient *http.Client
}

func NewProxyClient(_ string) *ProxyClient {
	return &ProxyClient{
		HTTPClient: &http.Client{Timeout: 12 * time.Second},
	}
}

func (pc *ProxyClient) doGet(base, host, endpoint string, params map[string]string) (*http.Response, error) {
	req, err := http.NewRequest("GET", base+endpoint, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	for k, v := range params {
		q.Add(k, v)
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("X-RapidAPI-Key", rapidAPIKey)
	req.Header.Set("X-RapidAPI-Host", host)
	return pc.HTTPClient.Do(req)
}

// SearchHotelDestinations returns city suggestions for hotel autocomplete via Hotels.com regions API.
func (pc *ProxyClient) SearchHotelDestinations(query string) ([]DestSuggestion, error) {
	resp, err := pc.doGet(hotelsBase, hotelsHost, "/v2/regions", map[string]string{
		"query":  query,
		"locale": "en_US",
		"domain": "US",
	})
	if err != nil {
		return nil, fmt.Errorf("hotel destination search failed: %w", err)
	}
	defer resp.Body.Close()

	var payload hcRegionResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode hotel destination response: %w", err)
	}

	results := make([]DestSuggestion, 0)
	for _, r := range payload.Data {
		if r.Type != "CITY" && r.Type != "NEIGHBORHOOD" && r.Type != "AIRPORT" {
			continue
		}
		hierarchy := r.HierarchyInfo.Country.Name
		results = append(results, DestSuggestion{
			EntityID:  r.GaiaID,
			Name:      r.RegionNames.DisplayName,
			Type:      strings.ToLower(r.Type),
			Hierarchy: hierarchy,
		})
		if len(results) >= 6 {
			break
		}
	}
	return results, nil
}

// SearchHotelDestination returns the first city gaiaId for a query.
func (pc *ProxyClient) SearchHotelDestination(query string) (string, error) {
	results, err := pc.SearchHotelDestinations(query)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no destination found for %q", query)
	}
	return results[0].EntityID, nil
}

// SearchAirports returns airport suggestions using Google Flights airport search.
func (pc *ProxyClient) SearchAirports(query string) ([]FlightDestSuggestion, error) {
	resp, err := pc.doGet(gfBase, gfHost, "/api/v1/searchAirport", map[string]string{"query": query})
	if err != nil {
		return nil, fmt.Errorf("airport search failed: %w", err)
	}
	defer resp.Body.Close()

	var payload gfAirportResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode airport response: %w", err)
	}

	var results []FlightDestSuggestion
	for _, group := range payload.Data {
		countryName := ""
		if idx := strings.LastIndex(group.Name, ", "); idx >= 0 {
			countryName = group.Name[idx+2:]
		}
		for _, a := range group.List {
			if a.Type != "airport" || len(a.ID) != 3 {
				continue
			}
			results = append(results, FlightDestSuggestion{
				SkyID:       a.ID,
				EntityID:    a.ID,
				Name:        a.Name,
				CityName:    a.City,
				CountryName: countryName,
				PlaceType:   "airport",
			})
		}
		if len(results) >= 8 {
			break
		}
	}
	return results, nil
}

// hotelTemplates define names, stars and pricing only; photos are generated dynamically per city.
var hotelTemplates = []struct {
	suffix string
	stars  int
	base   float64
	rating float64
	word   string
}{
	{"Grand Palace Hotel", 5, 220, 9.2, "Exceptional"},
	{"Boutique Centrale", 4, 135, 8.8, "Excellent"},
	{"The Riverside Suites", 4, 160, 8.5, "Excellent"},
	{"Luxury Tower & Spa", 5, 310, 9.5, "Exceptional"},
	{"City View Residences", 3, 88, 7.9, "Good"},
	{"Heritage Collection", 4, 175, 9.0, "Exceptional"},
	{"The Modern Stay", 4, 145, 8.6, "Excellent"},
	{"Skyline Boutique", 3, 99, 8.1, "Very Good"},
	{"Art Deco Suites", 4, 195, 8.9, "Excellent"},
	{"Garden Quarter Inn", 3, 75, 7.7, "Good"},
	{"Panorama Rooftop Hotel", 5, 280, 9.3, "Exceptional"},
	{"The Old Town Lodge", 3, 65, 8.0, "Very Good"},
}

// FetchHotels generates realistic mock listings for the given city.
// (Hotels.com free tier returns no usable data, so we simulate results locally.)
func (pc *ProxyClient) FetchHotels(city, regionID, checkIn, checkOut, adults, children, rooms string) ([]HotelData, error) {
	// Seed a deterministic-but-varied set per city
	seed := int64(0)
	for _, c := range city {
		seed = seed*31 + int64(c)
	}
	if seed < 0 {
		seed = -seed
	}

	cityQ := url.QueryEscape(city)
	hotels := make([]HotelData, 0, len(hotelTemplates))
	for i := range hotelTemplates {
		idx := (int(seed) + i) % len(hotelTemplates)
		tpl := hotelTemplates[idx]
		name := city + " " + tpl.suffix
		price := tpl.base + float64((int(seed)+i)%5)*11.0
		// Dynamic photo: Unsplash source URL searches for city+hotel so each result
		// shows a real photo relevant to that city. sig= ensures different images per card.
		sig := int(seed)%200 + i*17
		photo := fmt.Sprintf("https://source.unsplash.com/600x400/?hotel,luxury,%s&sig=%d", cityQ, sig)
		hotels = append(hotels, HotelData{
			HotelID:    int(seed%9000) + 1000 + i,
			HotelName:  name,
			Price:      price,
			Currency:   "USD",
			Rating:     tpl.rating,
			RatingWord: tpl.word,
			PhotoURL:   photo,
			Stars:      tpl.stars,
		})
	}
	return hotels, nil
}

// FetchTravelData is kept for the /travel-data JSON endpoint.
func (pc *ProxyClient) FetchTravelData() ([]HotelData, error) {
	return pc.FetchHotels("London", "2872", "2026-08-01", "2026-08-05", "2", "0", "1")
}

// FetchFlights searches flights via Google Flights API.
// fromEntityID and toEntityID are ignored (Google Flights uses IATA codes only).
func (pc *ProxyClient) FetchFlights(fromSkyID, _, toSkyID, _, date, returnDate, adults, children, cabinClass string) ([]FlightData, error) {
	if adults == "" {
		adults = "1"
	}
	if cabinClass == "" {
		cabinClass = "economy"
	}

	params := map[string]string{
		"departure_id":  fromSkyID,
		"arrival_id":    toSkyID,
		"outbound_date": date,
		"travel_class":  strings.ToUpper(cabinClass),
		"adults":        adults,
		"currency":      "USD",
		"search_type":   "best",
		"language_code": "en-US",
		"country_code":  "US",
	}
	if children != "" && children != "0" {
		params["children"] = children
	}
	if returnDate != "" {
		params["return_date"] = returnDate
	}

	resp, err := pc.doGet(gfBase, gfHost, "/api/v1/searchFlights", params)
	if err != nil {
		return nil, fmt.Errorf("flight search failed: %w", err)
	}
	defer resp.Body.Close()

	var payload gfFlightResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode flight response: %w", err)
	}
	if !payload.Status {
		return nil, fmt.Errorf("google flights returned no results")
	}

	all := append(payload.Data.Itineraries.TopFlights, payload.Data.Itineraries.OtherFlights...)
	flights := make([]FlightData, 0, len(all))
	for _, it := range all {
		if len(it.Flights) == 0 {
			continue
		}
		first := it.Flights[0]
		last := it.Flights[len(it.Flights)-1]
		flights = append(flights, FlightData{
			FromCode:        first.DepartureAirport.AirportCode,
			ToCode:          last.ArrivalAirport.AirportCode,
			DepartTime:      extractTime(it.DepartureTime),
			ArriveTime:      extractTime(it.ArrivalTime),
			DurationHours:   it.Duration.Raw / 60,
			DurationMinutes: it.Duration.Raw % 60,
			Airline:         first.Airline,
			AirlineLogo:     it.AirlineLogo,
			Price:           it.Price,
			Currency:        "USD",
			Stops:           len(it.Layovers),
		})
	}
	return flights, nil
}

// extractTime pulls the time from "15-06-2026 11:25 PM" and converts to 24h format ("23:25").
func extractTime(s string) string {
	// s format: "DD-MM-YYYY HH:MM AM/PM"
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return s
	}
	timePart := parts[1] // "11:25"
	ampm := ""
	if len(parts) >= 3 {
		ampm = strings.ToUpper(parts[2]) // "AM" or "PM"
	}
	if ampm == "" {
		return timePart // already 24h or unknown format
	}
	hm := strings.SplitN(timePart, ":", 2)
	if len(hm) != 2 {
		return timePart
	}
	hour := 0
	fmt.Sscanf(hm[0], "%d", &hour)
	if ampm == "PM" && hour != 12 {
		hour += 12
	} else if ampm == "AM" && hour == 12 {
		hour = 0
	}
	return fmt.Sprintf("%02d:%s", hour, hm[1])
}
