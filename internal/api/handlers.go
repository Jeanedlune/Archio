package api

import (
	"encoding/json"
	"net/http"

	"github.com/Jeanedlune/archio/internal/jobqueue"
	"github.com/Jeanedlune/archio/internal/kvstore"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

type Server struct {
	store    kvstore.Store
	queue    *jobqueue.Queue
	validate *validator.Validate
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

	if err := json.NewEncoder(w).Encode(jobs); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
