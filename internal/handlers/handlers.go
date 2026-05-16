package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"travel-proxy-service/internal/db"
	"travel-proxy-service/internal/proxy"
)

type TravelHandler struct {
	ProxyClient *proxy.ProxyClient
	Templates   *template.Template
	DB          *db.DB
}

type FlightLeg struct {
	Label   string
	FromSky string
	ToSky   string
	Date    string
	Flights []proxy.FlightData
}

type pageData struct {
	Tab          string
	City         string
	CityEntityID string
	Checkin      string
	Checkout     string
	Adults       string
	Children     string
	Rooms        string
	FromSkyID    string
	FromEntityID string
	ToSkyID      string
	ToEntityID   string
	FromCity     string
	ToCity       string
	Date         string
	ReturnDate   string
	TripType     string
	CabinClass   string
	Hotels       []proxy.HotelData
	Flights      []proxy.FlightData
	FlightLegs   []FlightLeg
	PickupCity   string
	PickupDate   string
	DropoffDate  string
	DriverAge    string
	Nights       int
	Error        string
}

func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "OK"})
}

func (h *TravelHandler) render(w http.ResponseWriter, data pageData) {
	tmpl := h.Templates.Lookup("index.html")
	if tmpl == nil {
		log.Println("template index.html not found")
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("template execution error: %v", err)
	}
}

func (h *TravelHandler) HomeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	city := r.URL.Query().Get("q")
	entityID := r.URL.Query().Get("entityId")
	if city == "" {
		h.render(w, pageData{Tab: "hotels"})
		return
	}

	checkin := r.URL.Query().Get("checkin")
	if checkin == "" {
		checkin = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	}
	checkout := r.URL.Query().Get("checkout")
	if checkout == "" {
		checkout = time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	}
	adults := r.URL.Query().Get("adults")
	children := r.URL.Query().Get("children")
	rooms := r.URL.Query().Get("rooms")
	if adults == "" { adults = "2" }
	if children == "" { children = "0" }
	if rooms == "" { rooms = "1" }

	nights := 4
	if t1, e1 := time.Parse("2006-01-02", checkin); e1 == nil {
		if t2, e2 := time.Parse("2006-01-02", checkout); e2 == nil {
			if d := int(t2.Sub(t1).Hours() / 24); d > 0 {
				nights = d
			}
		}
	}

	pd := pageData{Tab: "hotels", City: city, Checkin: checkin, Checkout: checkout, Adults: adults, Children: children, Rooms: rooms, Nights: nights}

	if entityID == "" {
		var err error
		entityID, err = h.ProxyClient.SearchHotelDestination(city)
		if err != nil {
			pd.Error = "Destination not found: " + city
			h.render(w, pd)
			return
		}
	}
	pd.CityEntityID = entityID

	hotels, err := h.ProxyClient.FetchHotels(city, entityID, checkin, checkout, adults, children, rooms)
	if err != nil {
		pd.Error = "Failed to load hotels. Please try again."
		h.render(w, pd)
		return
	}

	pd.Hotels = hotels
	h.render(w, pd)
}

func (h *TravelHandler) resolveEntityID(skyID string) string {
	airports, err := h.ProxyClient.SearchAirports(skyID)
	if err != nil {
		return ""
	}
	for _, a := range airports {
		if a.SkyID == skyID {
			return a.EntityID
		}
	}
	if len(airports) > 0 {
		return airports[0].EntityID
	}
	return ""
}

func (h *TravelHandler) FlightsHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	adults := q.Get("adults")
	children := q.Get("children")
	cabinClass := q.Get("cabinClass")
	tripType := q.Get("tripType")
	if adults == "" { adults = "1" }
	if cabinClass == "" { cabinClass = "economy" }

	returnDate := q.Get("returnDate")
	if returnDate != "" && tripType != "multi" {
		tripType = "round"
	} else if tripType == "" {
		tripType = "oneway"
	}

	// ── Multi-city ────────────────────────────────────────────────────────────
	if tripType == "multi" {
		pd := pageData{Tab: "flights", TripType: "multi", Adults: adults, Children: children, CabinClass: cabinClass}
		var legs []FlightLeg

		for i := 0; i < 6; i++ {
			fromSky := q.Get(fmt.Sprintf("leg%dfrom", i))
			toSky := q.Get(fmt.Sprintf("leg%dto", i))
			date := q.Get(fmt.Sprintf("leg%ddate", i))
			if fromSky == "" || toSky == "" || date == "" {
				break
			}
			fromEntity := q.Get(fmt.Sprintf("leg%dfromEntity", i))
			toEntity := q.Get(fmt.Sprintf("leg%dtoEntity", i))
			if fromEntity == "" { fromEntity = h.resolveEntityID(fromSky) }
			if toEntity == "" { toEntity = h.resolveEntityID(toSky) }

			flights, err := h.ProxyClient.FetchFlights(fromSky, fromEntity, toSky, toEntity, date, "", adults, children, cabinClass)
			leg := FlightLeg{
				Label:   fmt.Sprintf("%s → %s", fromSky, toSky),
				FromSky: fromSky,
				ToSky:   toSky,
				Date:    date,
			}
			if err == nil {
				leg.Flights = flights
			}
			legs = append(legs, leg)
		}
		pd.FlightLegs = legs
		h.render(w, pd)
		return
	}

	// ── One-way / Round-trip ──────────────────────────────────────────────────
	fromSkyID := q.Get("fromSky")
	fromEntityID := q.Get("fromEntity")
	toSkyID := q.Get("toSky")
	toEntityID := q.Get("toEntity")
	date := q.Get("date")

	if fromSkyID == "" || toSkyID == "" {
		h.render(w, pageData{
			Tab: "flights", TripType: "oneway",
			Adults: adults, Children: children, CabinClass: cabinClass,
		})
		return
	}

	if date == "" { date = time.Now().AddDate(0, 0, 1).Format("2006-01-02") }
	if fromEntityID == "" { fromEntityID = h.resolveEntityID(fromSkyID) }
	if toEntityID == "" { toEntityID = h.resolveEntityID(toSkyID) }

	pd := pageData{
		Tab: "flights", TripType: tripType,
		FromSkyID: fromSkyID, FromEntityID: fromEntityID,
		ToSkyID: toSkyID, ToEntityID: toEntityID,
		Date: date, ReturnDate: returnDate,
		Adults: adults, Children: children, CabinClass: cabinClass,
	}

	flights, err := h.ProxyClient.FetchFlights(fromSkyID, fromEntityID, toSkyID, toEntityID, date, returnDate, adults, children, cabinClass)
	if err != nil {
		pd.Error = "Failed to load flights. Please try again."
		h.render(w, pd)
		return
	}
	pd.Flights = flights
	if len(flights) > 0 {
		pd.FromCity = flights[0].FromCode
		pd.ToCity = flights[0].ToCode
	}
	h.render(w, pd)
}

func (h *TravelHandler) CarsHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pd := pageData{
		Tab:         "cars",
		PickupCity:  q.Get("pickup"),
		PickupDate:  q.Get("pickupDate"),
		DropoffDate: q.Get("dropoffDate"),
		DriverAge:   q.Get("age"),
	}
	if pd.DriverAge == "" {
		pd.DriverAge = "30"
	}
	if pd.PickupDate == "" {
		pd.PickupDate = time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	}
	if pd.DropoffDate == "" {
		pd.DropoffDate = time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	}
	h.render(w, pd)
}

func (h *TravelHandler) SuggestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}
	results, err := h.ProxyClient.SearchHotelDestinations(q)
	if err != nil {
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}
	json.NewEncoder(w).Encode(results)
}

func (h *TravelHandler) SuggestFlightHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}
	results, err := h.ProxyClient.SearchAirports(q)
	if err != nil {
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}
	json.NewEncoder(w).Encode(results)
}

func (h *TravelHandler) GetTravelDataHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	hotels, err := h.ProxyClient.FetchTravelData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := json.NewEncoder(w).Encode(hotels); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}
