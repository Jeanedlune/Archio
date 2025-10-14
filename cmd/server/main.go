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

	// Health check endpoints
	r.Get("/health", server.HealthCheck)
	r.Get("/ready", server.ReadinessCheck)

	// Metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	// Start the server with graceful shutdown
	srv := &http.Server{
		Addr:    config.Server.Port,
		Handler: r,
	}

	// Mark server as ready
	server.SetReady(true)
	log.Printf("Server marked as ready")

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
	log.Println("Received shutdown signal, starting graceful shutdown...")

	// Mark server as not ready immediately
	server.SetReady(false)
	log.Println("Server marked as not ready")

	// Create shutdown context with timeout
	shutdownTimeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Shutdown HTTP server (stops accepting new connections)
	log.Println("Shutting down HTTP server...")
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	} else {
		log.Println("HTTP server shut down successfully")
	}

	// Shutdown Raft node
	if raftNode != nil {
		log.Println("Shutting down Raft node...")
		if err := raftNode.Shutdown(); err != nil {
			log.Printf("Raft shutdown error: %v", err)
		} else {
			log.Println("Raft node shut down successfully")
		}
	}

	// Shutdown job queue (stops workers)
	log.Println("Shutting down job queue...")
	queue.Shutdown()
	log.Println("Job queue shut down successfully")

	// Close storage
	log.Println("Closing storage...")
	if err := store.Close(); err != nil {
		log.Printf("Error closing store: %v", err)
	} else {
		log.Println("Storage closed successfully")
	}

	log.Println("Graceful shutdown completed")
}
