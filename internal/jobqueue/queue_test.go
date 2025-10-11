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

// New tests to verify configurable retry behavior

func newQueueWithConfigRetries(maxRetries int, backoff time.Duration, shouldErr bool) (*Queue, *mockHandler, string) {
	cfg := &QueueConfig{
		WorkerCount:  1,
		QueueSize:    10,
		MaxRetries:   maxRetries,
		RetryBackoff: backoff,
	}
	q := NewQueueWithConfig(cfg)
	h := &mockHandler{shouldErr: shouldErr}
	q.RegisterHandler(testJobType, h)
	id := q.AddJob(testJobType, []byte(testPayload))
	return q, h, id
}

func TestJobRetryRespectsMaxRetries(t *testing.T) {
	// Handler selalu error, MaxRetries=2 => total attempts harus 2 dan status akhir failed
	q, _, id := newQueueWithConfigRetries(2, 10*time.Millisecond, true)

	// Tunggu cukup lama untuk dua attempt + satu backoff
	time.Sleep(100 * time.Millisecond)

	job, ok := q.GetJob(id)
	assertJobExists(t, ok)

	if job.Attempts != 2 {
		t.Fatalf("expected attempts=2, got %d", job.Attempts)
	}
	if job.Status != "failed" {
		t.Fatalf("expected final status 'failed', got %q", job.Status)
	}
}

func TestJobRetryBackoffRespected(t *testing.T) {
	// MaxRetries >=2 agar ada attempt ke-2; gunakan backoff yang cukup besar agar terukur
	backoff := 120 * time.Millisecond
	q, _, id := newQueueWithConfigRetries(2, backoff, true)

	var t1, t2 time.Time
	deadline := time.Now().Add(2 * time.Second)

	// Tunggu sampai attempts >= 1 (attempt pertama)
	for time.Now().Before(deadline) {
		job, ok := q.GetJob(id)
		if !ok {
			t.Fatal("job not found")
		}
		if job.Attempts >= 1 {
			t1 = time.Now()
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if t1.IsZero() {
		t.Fatal("did not observe first attempt in time")
	}

	// Tunggu sampai attempts >= 2 (setelah backoff)
	for time.Now().Before(deadline) {
		job, ok := q.GetJob(id)
		if !ok {
			t.Fatal("job not found")
		}
		if job.Attempts >= 2 {
			t2 = time.Now()
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if t2.IsZero() {
		t.Fatal("did not observe second attempt in time")
	}

	delta := t2.Sub(t1)
	// Longgar untuk flakiness: minimal 80% dari backoff dan maksimal 1s
	min := backoff - 30*time.Millisecond
	if min < 0 {
		min = 0
	}
	if delta < min {
		t.Fatalf("second attempt occurred too soon: got %v, want >= %v", delta, min)
	}
	if delta > time.Second {
		t.Fatalf("second attempt took too long: got %v", delta)
	}
}
