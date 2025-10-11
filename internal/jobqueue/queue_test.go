package jobqueue

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

const (
	testJobType    = "test"
	testPayload    = "test payload"
	testErrMsg     = "mock error"
	jobShouldExist = "Job should exist"
)

type mockHandler struct {
	mu        sync.Mutex
	called    bool
	job       *Job
	shouldErr bool
}

func (h *mockHandler) Handle(ctx context.Context, job *Job) error {
	h.mu.Lock()
	h.called = true
	h.job = job
	h.mu.Unlock()
	if h.shouldErr {
		return fmt.Errorf("%s", testErrMsg)
	}
	return nil
}

func assertJobExists(t *testing.T, exists bool) {
	t.Helper()
	if !exists {
		t.Fatal(jobShouldExist)
	}
}

func setupTestQueue(shouldErr bool) (*Queue, *mockHandler, string) {
	queue := NewQueue()
	handler := &mockHandler{shouldErr: shouldErr}
	queue.RegisterHandler(testJobType, handler)
	return queue, handler, queue.AddJob(testJobType, []byte(testPayload))
}

func TestAddAndGetJob(t *testing.T) {
	queue := NewQueue()
	payload := []byte(testPayload)

	id := queue.AddJob(testJobType, payload)
	if id == "" {
		t.Fatal("Expected non-empty job ID")
	}

	job, exists := queue.GetJob(id)
	assertJobExists(t, exists)

	if job.Type != testJobType {
		t.Errorf("Got job type %s, want %s", job.Type, testJobType)
	}
	if string(job.Payload) != testPayload {
		t.Errorf("Got payload %s, want %s", string(job.Payload), testPayload)
	}
}

func TestJobProcessing(t *testing.T) {
	queue, handler, id := setupTestQueue(false)

	// Wait for job processing
	time.Sleep(100 * time.Millisecond)

	handler.mu.Lock()
	called := handler.called
	job := handler.job
	handler.mu.Unlock()

	if !called {
		t.Error("Handler should have been called")
	}
	if job == nil {
		t.Fatal("Handler should have received a job")
	}
	if job.ID != id {
		t.Errorf("Got job ID %s, want %s", job.ID, id)
	}

	queueJob, exists := queue.GetJob(id)
	assertJobExists(t, exists)

	if queueJob.Status != "completed" {
		t.Errorf("Got status %s, want completed", queueJob.Status)
	}
}

func TestJobRetry(t *testing.T) {
	queue, _, id := setupTestQueue(true)

	// Wait for initial processing and retry
	time.Sleep(200 * time.Millisecond)

	job, exists := queue.GetJob(id)
	assertJobExists(t, exists)

	if job.Attempts == 0 {
		t.Error("Job should have been attempted at least once")
	}
	if job.Error == "" {
		t.Error("Job should have error message")
	}
}
