package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

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

	// Initialize Raft if enabled
	var raftNode *kvstore.RaftNode
	if config.Raft.Enabled {
		raftDir := filepath.Join(config.Storage.DataDir, "raft")
		if err := os.MkdirAll(raftDir, 0o755); err != nil {
			log.Fatalf("Failed to create raft data directory: %v", err)
		}
		rn, err := kvstore.NewRaftNode(store, raftDir, config.Raft.BindAddr, config.Raft.Bootstrap)
		if err != nil {
			log.Fatalf("Failed to initialize Raft node: %v", err)
		}
		raftNode = rn
	}

	// Initialize job queue with configuration and persistence
	queueConfig := &jobqueue.QueueConfig{
		WorkerCount:  config.JobQueue.WorkerCount,
		QueueSize:    config.JobQueue.QueueSize,
		MaxRetries:   config.JobQueue.MaxRetries,
		RetryBackoff: config.JobQueue.RetryBackoff,
	}
	queue := jobqueue.NewQueueWithConfig(queueConfig, store)
	log.Printf("Job queue initialized with persistence enabled")
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
		r.Get("/", server.ListJobs)
		r.Get("/{id}", server.GetJob)
		r.Delete("/{id}", server.DeleteJob)
	})

	// Metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	// Start the server with graceful shutdown
	srv := &http.Server{Addr: config.Server.Port, Handler: r}

	go func() {
		log.Printf("Starting server on %s...", config.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for termination signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP server Shutdown: %v", err)
	}
	if raftNode != nil {
		if err := raftNode.Shutdown(); err != nil {
			log.Printf("Raft shutdown error: %v", err)
		}
	}
	queue.Shutdown()
	log.Println("Job queue shut down")
	if err := store.Close(); err != nil {
		log.Printf("Error closing store: %v", err)
	}
}
