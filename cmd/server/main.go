package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Jeanedlune/archio/internal/api"
	"github.com/Jeanedlune/archio/internal/jobqueue"
	"github.com/Jeanedlune/archio/internal/kvstore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll("data/kvstore", 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Initialize components
	var store kvstore.Store
	store, err := kvstore.NewBadgerStore("data/kvstore")
	if err != nil {
		log.Printf("Failed to initialize BadgerStore: %v, falling back to MemoryStore", err)
		store = kvstore.NewMemoryStore()
	}
	defer store.Close()

	queue := jobqueue.NewQueue()
	server := api.NewServer(store, queue)

	// Create router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.AllowContentType("application/json"))

	// KV Store routes
	r.Route("/kv", func(r chi.Router) {
		r.Post("/{key}", server.SetValue)
		r.Get("/{key}", server.GetValue)
	})

	// Job Queue routes
	r.Route("/jobs", func(r chi.Router) {
		r.Post("/", server.CreateJob)
		r.Get("/{id}", server.GetJob)
	})

	// Metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	// Start the server
	log.Println("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}
}
