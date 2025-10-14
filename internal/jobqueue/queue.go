package jobqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Job represents a task to be executed
type Job struct {
	ID       string    `json:"id"`
	Type     string    `json:"type"`
	Payload  []byte    `json:"payload"`
	Status   string    `json:"status"`
	Error    string    `json:"error,omitempty"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
	Attempts int       `json:"attempts"`
}

// Store defines the interface for persistent storage
type Store interface {
	Set(key string, value []byte) error
	Get(key string) ([]byte, bool, error)
	Delete(key string) error
	ForEach(fn func(key string, value []byte) error) error
}

// Queue manages the job queue system
type Queue struct {
	mu        sync.RWMutex
	jobs      map[string]*Job
	tasks     chan *Job
	processor *JobProcessor
	ctx       context.Context
	cancel    context.CancelFunc
	config    *QueueConfig
	store     Store // Persistent storage for jobs
}

// QueueConfig holds configuration for the job queue
type QueueConfig struct {
	WorkerCount  int
	QueueSize    int
	MaxRetries   int
	RetryBackoff time.Duration
}

// DefaultConfig returns default queue configuration
func DefaultConfig() *QueueConfig {
	return &QueueConfig{
		WorkerCount:  5,
		QueueSize:    100,
		MaxRetries:   3,
		RetryBackoff: time.Second * 5,
	}
}

// NewQueue creates a new job queue instance without persistence
func NewQueue() *Queue {
	return NewQueueWithConfig(DefaultConfig(), nil)
}

// NewQueueWithStore creates a new job queue with persistence
func NewQueueWithStore(store Store) *Queue {
	return NewQueueWithConfig(DefaultConfig(), store)
}

// NewQueueWithConfig creates a new job queue with custom configuration and optional persistence
func NewQueueWithConfig(config *QueueConfig, store Store) *Queue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue{
		jobs:      make(map[string]*Job),
		tasks:     make(chan *Job, config.QueueSize),
		processor: NewJobProcessor(),
		ctx:       ctx,
		cancel:    cancel,
		config:    config,
		store:     store,
	}

	// Register default handlers
	q.processor.RegisterHandler("email", &EmailJobHandler{})
	q.processor.RegisterHandler("data_processing", &DataProcessingHandler{})

	// Recover jobs from persistent storage if available
	if store != nil {
		if err := q.recoverJobs(); err != nil {
			log.Printf("Warning: Failed to recover jobs from storage: %v", err)
		}
	}

	// Start worker pool
	go q.startWorkers(config.WorkerCount)
	return q
}

// RegisterHandler registers a new job handler
func (q *Queue) RegisterHandler(jobType string, handler JobHandler) {
	q.processor.RegisterHandler(jobType, handler)
}

// startWorkers initializes a pool of worker goroutines
func (q *Queue) startWorkers(count int) {
	for i := 0; i < count; i++ {
		go q.worker()
	}
}

// worker processes jobs from the queue
func (q *Queue) worker() {
	for {
		select {
		case <-q.ctx.Done():
			return
		case job := <-q.tasks:
			q.processJob(job)
		}
	}
}

// processJob handles the processing of a single job
func (q *Queue) processJob(job *Job) {
	q.mu.Lock()
	job.Status = "processing"
	job.Attempts++
	job.Updated = time.Now()
	q.saveJob(job) // Persist state change
	q.mu.Unlock()

	err := q.processor.Process(q.ctx, job)

	q.mu.Lock()
	defer q.mu.Unlock()

	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		if job.Attempts < q.config.MaxRetries {
			job.Status = "pending"
			time.AfterFunc(q.config.RetryBackoff, func() {
				q.tasks <- job
			})
		}
	} else {
		job.Status = "completed"
	}
	job.Updated = time.Now()
	q.saveJob(job) // Persist final state
}

// AddJob adds a new job to the queue
func (q *Queue) AddJob(jobType string, payload []byte) string {
	job := &Job{
		ID:      generateID(),
		Type:    jobType,
		Payload: payload,
		Status:  "pending",
		Created: time.Now(),
		Updated: time.Now(),
	}

	q.mu.Lock()
	q.jobs[job.ID] = job
	q.saveJob(job) // Persist new job
	q.mu.Unlock()

	q.tasks <- job
	return job.ID
}

// GetJob retrieves a job by ID
func (q *Queue) GetJob(id string) (*Job, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	job, exists := q.jobs[id]
	if !exists {
		return nil, false
	}
	// Return a copy to avoid exposing internal mutable state and prevent data races
	jobCopy := *job
	return &jobCopy, true
}

// DeleteJob removes a job from the queue and persistent storage
func (q *Queue) DeleteJob(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.jobs[id]; !exists {
		return fmt.Errorf("job %s not found", id)
	}

	delete(q.jobs, id)
	q.deleteJob(id)
	return nil
}

// ListJobs returns all jobs in the queue
func (q *Queue) ListJobs() []*Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	jobs := make([]*Job, 0, len(q.jobs))
	for _, job := range q.jobs {
		jobCopy := *job
		jobs = append(jobs, &jobCopy)
	}
	return jobs
}

// generateID creates a unique job ID using UUID
func generateID() string {
	return uuid.New().String()
}

// jobKeyPrefix is the prefix for job keys in the store
const jobKeyPrefix = "job:"

// saveJob persists a job to the store (caller must hold lock)
func (q *Queue) saveJob(job *Job) {
	if q.store == nil {
		return
	}

	data, err := json.Marshal(job)
	if err != nil {
		log.Printf("Error marshaling job %s: %v", job.ID, err)
		return
	}

	key := jobKeyPrefix + job.ID
	if err := q.store.Set(key, data); err != nil {
		log.Printf("Error saving job %s to store: %v", job.ID, err)
	}
}

// deleteJob removes a job from persistent storage (caller must hold lock)
func (q *Queue) deleteJob(jobID string) {
	if q.store == nil {
		return
	}

	key := jobKeyPrefix + jobID
	if err := q.store.Delete(key); err != nil {
		log.Printf("Error deleting job %s from store: %v", jobID, err)
	}
}

// recoverJobs loads jobs from persistent storage on startup
func (q *Queue) recoverJobs() error {
	if q.store == nil {
		return nil
	}

	recoveredCount := 0
	restartedCount := 0

	err := q.store.ForEach(func(key string, value []byte) error {
		// Only process job keys
		if !strings.HasPrefix(key, jobKeyPrefix) {
			return nil
		}

		var job Job
		if err := json.Unmarshal(value, &job); err != nil {
			log.Printf("Error unmarshaling job from key %s: %v", key, err)
			return nil // Continue with other jobs
		}

		q.mu.Lock()
		q.jobs[job.ID] = &job
		q.mu.Unlock()
		recoveredCount++

		// Re-queue pending or processing jobs
		if job.Status == "pending" || job.Status == "processing" {
			// Reset processing jobs to pending to avoid duplicate processing
			if job.Status == "processing" {
				q.mu.Lock()
				job.Status = "pending"
				job.Updated = time.Now()
				q.saveJob(&job)
				q.mu.Unlock()
			}
			q.tasks <- &job
			restartedCount++
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error iterating over stored jobs: %w", err)
	}

	log.Printf("Job recovery complete: %d jobs recovered, %d jobs restarted", recoveredCount, restartedCount)
	return nil
}

// Shutdown gracefully shuts down the queue
func (q *Queue) Shutdown() {
	q.cancel()
}
