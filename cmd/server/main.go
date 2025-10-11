package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Jeanedlune/archio/configs"
	"github.com/Jeanedlune/archio/internal/api"
	"github.com/Jeanedlune/archio/internal/jobqueue"
	"github.com/Jeanedlune/archio/internal/kvstore"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Parse command line flags
	configFile := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Load configuration
	config, err := configs.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create data directory if it doesn't exist
	dataDir := filepath.Join(config.Storage.DataDir, "kvstore")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Initialize components
	var store kvstore.Store
	if config.Storage.Type == "badger" {
		store, err = kvstore.NewBadgerStore(dataDir)
		if err != nil {
			log.Printf("Failed to initialize BadgerStore: %v, falling back to MemoryStore", err)
			store = kvstore.NewMemoryStore()
		}
	} else {
		store = kvstore.NewMemoryStore()
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("Error closing store: %v", err)
		}
	}()

	// Initialize job queue with configuration
	queueConfig := &jobqueue.QueueConfig{
		WorkerCount:  config.JobQueue.WorkerCount,
		QueueSize:    config.JobQueue.QueueSize,
		MaxRetries:   config.JobQueue.MaxRetries,
		RetryBackoff: config.JobQueue.RetryBackoff,
	}
	queue := jobqueue.NewQueueWithConfig(queueConfig)
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
	log.Printf("Starting server on %s...", config.Server.Port)
	if err := http.ListenAndServe(config.Server.Port, r); err != nil {
		log.Fatal(err)
	}
}
