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

// Helpers for polling in tests
func waitForMinAttempts(t *testing.T, q *Queue, id string, minAttempts int, deadline time.Time) time.Time {
	t.Helper()
	for time.Now().Before(deadline) {
		job, ok := q.GetJob(id)
		if !ok {
			t.Fatal("job not found")
		}
		if job.Attempts >= minAttempts {
			return job.Updated
		}
		time.Sleep(5 * time.Millisecond)
	}
	return time.Time{}
}

func waitForStatus(t *testing.T, q *Queue, id string, status string, deadline time.Time) (*Job, bool) {
	t.Helper()
	for time.Now().Before(deadline) {
		job, ok := q.GetJob(id)
		if ok && job.Status == status {
			return job, true
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, ok := q.GetJob(id)
	return job, ok
}

// New tests to verify configurable retry behavior
func newQueueWithConfigRetries(maxRetries int, backoff time.Duration, shouldErr bool) (*Queue, *mockHandler, string) {
	cfg := &QueueConfig{
		WorkerCount:  1,
		QueueSize:    10,
		MaxRetries:   maxRetries,
		RetryBackoff: backoff,
	}
	q := NewQueueWithConfig(cfg, nil)
	h := &mockHandler{shouldErr: shouldErr}
	q.RegisterHandler(testJobType, h)
	id := q.AddJob(testJobType, []byte(testPayload))
	return q, h, id
}

func TestJobRetryRespectsMaxRetries(t *testing.T) {
	// Handler always errors, MaxRetries=2 => total attempts should be 2 and final status failed
	q, _, id := newQueueWithConfigRetries(2, 10*time.Millisecond, true)

	deadline := time.Now().Add(500 * time.Millisecond)
	job, ok := waitForStatus(t, q, id, "failed", deadline)
	assertJobExists(t, ok)

	if job.Status != "failed" {
		t.Fatalf("expected final status 'failed', got %q", job.Status)
	}
	if job.Attempts != 2 {
		t.Fatalf("expected attempts=2, got %d", job.Attempts)
	}
}

func TestJobRetryBackoffRespected(t *testing.T) {
	// MaxRetries >=2 to ensure second attempt; use a measurable backoff
	backoff := 120 * time.Millisecond
	timingTolerance := 30 * time.Millisecond
	q, _, id := newQueueWithConfigRetries(2, backoff, true)

	deadline := time.Now().Add(2 * time.Second)
	t1 := waitForMinAttempts(t, q, id, 1, deadline)
	if t1.IsZero() {
		t.Fatal("did not observe first attempt in time")
	}
	t2 := waitForMinAttempts(t, q, id, 2, deadline)
	if t2.IsZero() {
		t.Fatal("did not observe second attempt in time")
	}

	delta := t2.Sub(t1)
	min := backoff - timingTolerance
	if min < 0 {
		min = 0
	}
	if delta < min {
		t.Fatalf("second attempt occurred too soon: got %v, want >= %v", delta, min)
	}
	if delta > backoff*2 {
		t.Fatalf("second attempt took too long: got %v, want <= %v", delta, backoff*2)
	}
}
