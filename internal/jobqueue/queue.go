package jobqueue

import (
	"context"
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

// Queue manages the job queue system
type Queue struct {
	mu        sync.RWMutex
	jobs      map[string]*Job
	tasks     chan *Job
	processor *JobProcessor
	ctx       context.Context
	cancel    context.CancelFunc
	config    *QueueConfig
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

// NewQueue creates a new job queue instance
func NewQueue() *Queue {
	return NewQueueWithConfig(DefaultConfig())
}

// NewQueueWithConfig creates a new job queue with custom configuration
func NewQueueWithConfig(config *QueueConfig) *Queue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue{
		jobs:      make(map[string]*Job),
		tasks:     make(chan *Job, config.QueueSize),
		processor: NewJobProcessor(),
		ctx:       ctx,
		cancel:    cancel,
		config:    config,
	}

	// Register default handlers
	q.processor.RegisterHandler("email", &EmailJobHandler{})
	q.processor.RegisterHandler("data_processing", &DataProcessingHandler{})

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

// generateID creates a unique job ID using UUID
func generateID() string {
	return uuid.New().String()
}
