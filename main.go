package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var sessionManager *scs.SessionManager

func getHandler(w http.ResponseWriter, r *http.Request) {
	visited := sessionManager.GetString(r.Context(), "visited")
	component := page(visited)
	component.Render(r.Context(), w)
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	if r.Form.Has("country") {
		country := r.Form.Get("country")
		currentVisited := sessionManager.GetString(r.Context(), "visited")

		if !strings.Contains(currentVisited, country) {
			currentVisited += fmt.Sprintf(",%s", country)
		}

		log.Print(currentVisited)
		sessionManager.Put(r.Context(), "visited", currentVisited)
	}

	getHandler(w, r)
}

func main() {
	// Initialize the session.
	//
	sessionManager = scs.New()
	sessionManager.Lifetime = 24 * time.Hour

	mux := http.NewServeMux()

	// Handle POST and GET requests.
	//
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postHandler(w, r)
			return
		}
		log.Print("Received request")
		getHandler(w, r)
	})

	// Include the static content.
	//
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	// Add metrics endpoint.
	//
	mux.Handle("/metrics", promhttp.Handler())

	// Add the middleware.
	//
	muxWithSessionMiddleware := sessionManager.LoadAndSave(mux)

	// Start the server.
	//
	fmt.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", muxWithSessionMiddleware); err != nil {
		log.Printf("error listening: %v", err)
	}
}
