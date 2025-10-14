package api

import (
	"encoding/json"
	"net/http"
	"sync/atomic"

	"github.com/Jeanedlune/archio/internal/jobqueue"
	"github.com/Jeanedlune/archio/internal/kvstore"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type Server struct {
	store    kvstore.Store
	queue    *jobqueue.Queue
	validate *validator.Validate
	ready    atomic.Bool
}

func NewServer(store kvstore.Store, queue *jobqueue.Queue) *Server {
	return &Server{
		store:    store,
		queue:    queue,
		validate: validator.New(),
	}
}

// KV Store handlers
type KeyValueRequest struct {
	Value string `json:"value" validate:"required"`
}

func (s *Server) SetValue(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var req KeyValueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.validate.Struct(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.Set(key, []byte(req.Value)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) GetValue(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	value, exists, err := s.store.Get(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}

	if err := json.NewEncoder(w).Encode(map[string]string{
		"value": string(value),
	}); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

// Job Queue handlers
type JobRequest struct {
	Type    string `json:"type" validate:"required"`
	Payload string `json:"payload" validate:"required"`
}

type JobResponse struct {
	JobID string `json:"job_id"`
}

func (s *Server) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req JobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.validate.Struct(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	jobID := s.queue.AddJob(req.Type, []byte(req.Payload))

	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(JobResponse{JobID: jobID}); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (s *Server) GetJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	job, exists := s.queue.GetJob(jobID)
	if !exists {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	if err := json.NewEncoder(w).Encode(job); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (s *Server) DeleteJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	if err := s.queue.DeleteJob(jobID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs := s.queue.ListJobs()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(jobs); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

// Health check handlers

// SetReady marks the server as ready to accept traffic
func (s *Server) SetReady(ready bool) {
	s.ready.Store(ready)
}

// HealthCheck returns basic health status
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// ReadinessCheck returns readiness status
func (s *Server) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "not ready",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ready",
	})
}
