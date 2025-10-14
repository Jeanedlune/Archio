package jobqueue

import (
	"context"
	"fmt"
	"sync"
)

// JobHandler defines the interface for processing different types of jobs
type JobHandler interface {
	Handle(ctx context.Context, job *Job) error
}

// JobProcessor manages different job handlers
type JobProcessor struct {
	mu       sync.RWMutex
	handlers map[string]JobHandler
}

// NewJobProcessor creates a new job processor
func NewJobProcessor() *JobProcessor {
	return &JobProcessor{
		handlers: make(map[string]JobHandler),
	}
}

// RegisterHandler registers a handler for a specific job type
func (p *JobProcessor) RegisterHandler(jobType string, handler JobHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[jobType] = handler
}

// Process processes a job using the appropriate handler
func (p *JobProcessor) Process(ctx context.Context, job *Job) error {
	p.mu.RLock()
	handler, exists := p.handlers[job.Type]
	p.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no handler registered for job type: %s", job.Type)
	}
	return handler.Handle(ctx, job)
}

// Example handlers

// EmailJobHandler handles email jobs
type EmailJobHandler struct{}

func (h *EmailJobHandler) Handle(ctx context.Context, job *Job) error {
	// TODO: Implement actual email sending
	fmt.Printf("Sending email with payload: %s\n", string(job.Payload))
	return nil
}

// DataProcessingHandler handles data processing jobs
type DataProcessingHandler struct{}

func (h *DataProcessingHandler) Handle(ctx context.Context, job *Job) error {
	// TODO: Implement actual data processing
	fmt.Printf("Processing data with payload: %s\n", string(job.Payload))
	return nil
}
