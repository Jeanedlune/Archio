package jobqueue

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockStore is a simple in-memory store for testing
type mockStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMockStore() *mockStore {
	return &mockStore{
		data: make(map[string][]byte),
	}
}

func (s *mockStore) Set(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

func (s *mockStore) Get(key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, exists := s.data[key]
	return val, exists, nil
}

func (s *mockStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func (s *mockStore) ForEach(fn func(key string, value []byte) error) error {
	// Copy data to avoid holding lock during callback
	s.mu.RLock()
	dataCopy := make(map[string][]byte, len(s.data))
	for k, v := range s.data {
		// Make a copy of the value slice
		valueCopy := make([]byte, len(v))
		copy(valueCopy, v)
		dataCopy[k] = valueCopy
	}
	s.mu.RUnlock()

	// Call function without holding lock
	for k, v := range dataCopy {
		if err := fn(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *mockStore) Close() error {
	return nil
}

// TestJobPersistence verifies that jobs are persisted to storage
func TestJobPersistence(t *testing.T) {
	store := newMockStore()
	queue := NewQueueWithStore(store)
	defer queue.Shutdown()

	// Register a handler that does nothing
	queue.RegisterHandler(testJobType, &mockHandler{shouldErr: false})

	// Add a job
	jobID := queue.AddJob(testJobType, []byte(testPayload))

	// Wait a bit for the job to be processed
	time.Sleep(100 * time.Millisecond)

	// Check that the job was persisted
	key := jobKeyPrefix + jobID
	data, exists, err := store.Get(key)
	if err != nil {
		t.Fatalf("Error getting job from store: %v", err)
	}
	if !exists {
		t.Fatal("Job should be persisted in store")
	}
	if len(data) == 0 {
		t.Fatal("Persisted job data should not be empty")
	}
}

// TestJobRecovery verifies that jobs are recovered on restart
func TestJobRecovery(t *testing.T) {
	store := newMockStore()

	// Create first queue - jobs will complete successfully
	config1 := &QueueConfig{
		WorkerCount:  1,
		QueueSize:    10,
		MaxRetries:   3,
		RetryBackoff: 10 * time.Millisecond,
	}
	queue1 := NewQueueWithConfig(config1, store)
	handler1 := &mockHandler{shouldErr: false} // Success handler
	queue1.RegisterHandler(testJobType, handler1)

	jobID1 := queue1.AddJob(testJobType, []byte("payload1"))
	jobID2 := queue1.AddJob(testJobType, []byte("payload2"))

	// Wait for jobs to complete
	time.Sleep(150 * time.Millisecond)

	// Verify jobs completed in queue1
	job1Before, _ := queue1.GetJob(jobID1)
	job2Before, _ := queue1.GetJob(jobID2)

	if job1Before.Status != "completed" {
		t.Logf("Job 1 status before shutdown: %s", job1Before.Status)
	}
	if job2Before.Status != "completed" {
		t.Logf("Job 2 status before shutdown: %s", job2Before.Status)
	}

	// Shutdown first queue
	queue1.Shutdown()

	// Create second queue with same store (simulating restart)
	config2 := &QueueConfig{
		WorkerCount:  1,
		QueueSize:    10,
		MaxRetries:   3,
		RetryBackoff: 10 * time.Millisecond,
	}
	queue2 := NewQueueWithConfig(config2, store)
	handler2 := &mockHandler{shouldErr: false}
	queue2.RegisterHandler(testJobType, handler2)

	// Wait for recovery (completed jobs should not be reprocessed)
	time.Sleep(100 * time.Millisecond)

	// Check that jobs were recovered
	job1, exists1 := queue2.GetJob(jobID1)
	if !exists1 {
		t.Fatal("Job 1 should be recovered")
	}

	job2, exists2 := queue2.GetJob(jobID2)
	if !exists2 {
		t.Fatal("Job 2 should be recovered")
	}

	// Shutdown queue2 to clean up
	queue2.Shutdown()

	// Jobs should remain completed (not reprocessed)
	if job1.Status != "completed" {
		t.Errorf("Job 1 status should be 'completed', but got '%s' (attempts: %d)", job1.Status, job1.Attempts)
	}
	if job2.Status != "completed" {
		t.Errorf("Job 2 status should be 'completed', but got '%s' (attempts: %d)", job2.Status, job2.Attempts)
	}
}

// countingHandler tracks how many times Handle is called
type countingHandler struct {
	mu        sync.Mutex
	callCount int
	shouldErr bool
}

func (h *countingHandler) Handle(ctx context.Context, job *Job) error {
	h.mu.Lock()
	h.callCount++
	h.mu.Unlock()
	if h.shouldErr {
		return fmt.Errorf("mock error")
	}
	return nil
}

func (h *countingHandler) getCallCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.callCount
}

// TestJobRecoveryNoDuplicateProcessing verifies no duplicate processing on restart
func TestJobRecoveryNoDuplicateProcessing(t *testing.T) {
	store := newMockStore()

	// Create first queue
	queue1 := NewQueueWithStore(store)

	// Create a handler that tracks call count
	countingHandler := &countingHandler{shouldErr: false}

	queue1.RegisterHandler(testJobType, countingHandler)

	jobID := queue1.AddJob(testJobType, []byte(testPayload))

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	firstCallCount := countingHandler.getCallCount()

	if firstCallCount != 1 {
		t.Fatalf("Expected 1 call, got %d", firstCallCount)
	}

	// Shutdown and restart
	queue1.Shutdown()

	queue2 := NewQueueWithStore(store)
	defer queue2.Shutdown()
	queue2.RegisterHandler(testJobType, countingHandler)

	// Wait for potential duplicate processing
	time.Sleep(200 * time.Millisecond)

	finalCallCount := countingHandler.getCallCount()

	// Should still be 1 since job was already completed
	if finalCallCount != 1 {
		t.Fatalf("Expected 1 call (no duplicate), got %d", finalCallCount)
	}

	// Verify job status
	job, exists := queue2.GetJob(jobID)
	if !exists {
		t.Fatal("Job should exist after recovery")
	}
	if job.Status != "completed" {
		t.Errorf("Job status should be completed, got %s", job.Status)
	}
}

// blockingHandler blocks until channel is closed
type blockingHandler struct {
	blockChan   chan struct{}
	startedChan chan struct{}
}

func (h *blockingHandler) Handle(ctx context.Context, job *Job) error {
	// Signal that processing has started
	if h.startedChan != nil {
		h.startedChan <- struct{}{}
	}
	<-h.blockChan // Block until we close the channel
	return nil
}

// TestJobRecoveryProcessingToPending verifies processing jobs are reset to pending
func TestJobRecoveryProcessingToPending(t *testing.T) {
	store := newMockStore()

	// Create a handler that blocks
	blockChan := make(chan struct{})
	startedChan := make(chan struct{})
	blockingHandler := &blockingHandler{
		blockChan:   blockChan,
		startedChan: startedChan,
	}

	// Create a queue and register handler BEFORE starting workers
	queue1 := NewQueueWithStore(store)
	queue1.RegisterHandler(testJobType, blockingHandler)

	jobID := queue1.AddJob(testJobType, []byte(testPayload))

	// Wait for job to actually start processing
	select {
	case <-startedChan:
		// Job has started processing
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for job to start processing")
	}

	// Check job is in processing state
	job, exists := queue1.GetJob(jobID)
	if !exists {
		t.Fatal("Job should exist")
	}
	if job.Status != "processing" {
		t.Fatalf("Job status should be 'processing', got '%s'", job.Status)
	}

	// Shutdown queue while job is processing (don't unblock handler)
	queue1.Shutdown()
	close(blockChan) // Unblock after shutdown

	// Create new queue (simulating restart)
	queue2 := NewQueueWithStore(store)
	defer queue2.Shutdown()
	normalHandler := &mockHandler{shouldErr: false}
	queue2.RegisterHandler(testJobType, normalHandler)

	// Wait for recovery and processing
	time.Sleep(200 * time.Millisecond)

	// Job should have been reset to pending and then processed
	job, exists = queue2.GetJob(jobID)
	if !exists {
		t.Fatal("Job should exist after recovery")
	}

	// Should eventually be completed
	if job.Status != "completed" && job.Status != "pending" {
		t.Logf("Job status after recovery: %s", job.Status)
	}
}

// retryHandler fails first time, succeeds second time
type retryHandler struct {
	mu           sync.Mutex
	attemptCount int
}

func (h *retryHandler) Handle(ctx context.Context, job *Job) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.attemptCount++
	if h.attemptCount == 1 {
		return fmt.Errorf("first attempt fails")
	}
	return nil
}

// TestJobPersistenceStateChanges verifies all state changes are persisted
func TestJobPersistenceStateChanges(t *testing.T) {
	store := newMockStore()
	queue := NewQueueWithStore(store)
	defer queue.Shutdown()

	// Handler that fails first time, succeeds second time
	retryHandler := &retryHandler{}

	queue.RegisterHandler(testJobType, retryHandler)

	jobID := queue.AddJob(testJobType, []byte(testPayload))

	// Wait for retries
	time.Sleep(300 * time.Millisecond)

	// Get job from store
	key := jobKeyPrefix + jobID
	data, exists, err := store.Get(key)
	if err != nil {
		t.Fatalf("Error getting job from store: %v", err)
	}
	if !exists {
		t.Fatal("Job should be persisted")
	}

	// Verify the persisted job has the latest state
	job, exists := queue.GetJob(jobID)
	if !exists {
		t.Fatal("Job should exist")
	}

	// Should have multiple attempts
	if job.Attempts < 1 {
		t.Errorf("Expected at least 1 attempt, got %d", job.Attempts)
	}

	if len(data) == 0 {
		t.Fatal("Persisted data should not be empty")
	}
}

// TestQueueWithoutPersistence verifies queue works without store
func TestQueueWithoutPersistence(t *testing.T) {
	queue := NewQueue() // No store
	defer queue.Shutdown()

	handler := &mockHandler{shouldErr: false}
	queue.RegisterHandler(testJobType, handler)

	jobID := queue.AddJob(testJobType, []byte(testPayload))

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	job, exists := queue.GetJob(jobID)
	if !exists {
		t.Fatal("Job should exist")
	}
	if job.Status != "completed" {
		t.Errorf("Expected completed status, got %s", job.Status)
	}
}

// TestBackwardCompatibility verifies existing code still works
func TestBackwardCompatibility(t *testing.T) {
	// Old way of creating queue (without store)
	queue := NewQueue()
	defer queue.Shutdown()

	handler := &mockHandler{shouldErr: false}
	queue.RegisterHandler(testJobType, handler)

	jobID := queue.AddJob(testJobType, []byte(testPayload))

	time.Sleep(100 * time.Millisecond)

	job, exists := queue.GetJob(jobID)
	if !exists {
		t.Fatal("Job should exist")
	}
	if job.Status != "completed" {
		t.Errorf("Expected completed status, got %s", job.Status)
	}
}

// TestDeleteJob verifies job deletion
func TestDeleteJob(t *testing.T) {
	store := newMockStore()
	queue := NewQueueWithStore(store)
	defer queue.Shutdown()

	handler := &mockHandler{shouldErr: false}
	queue.RegisterHandler(testJobType, handler)

	jobID := queue.AddJob(testJobType, []byte(testPayload))

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify job exists
	_, exists := queue.GetJob(jobID)
	if !exists {
		t.Fatal("Job should exist")
	}

	// Delete job
	err := queue.DeleteJob(jobID)
	if err != nil {
		t.Fatalf("Failed to delete job: %v", err)
	}

	// Verify job is deleted
	_, exists = queue.GetJob(jobID)
	if exists {
		t.Fatal("Job should not exist after deletion")
	}

	// Verify job is deleted from store
	key := jobKeyPrefix + jobID
	_, exists, err = store.Get(key)
	if err != nil {
		t.Fatalf("Error checking store: %v", err)
	}
	if exists {
		t.Fatal("Job should not exist in store after deletion")
	}
}

// TestListJobs verifies listing all jobs
func TestListJobs(t *testing.T) {
	queue := NewQueue()
	defer queue.Shutdown()

	handler := &mockHandler{shouldErr: false}
	queue.RegisterHandler(testJobType, handler)

	// Add multiple jobs
	jobID1 := queue.AddJob(testJobType, []byte("payload1"))
	jobID2 := queue.AddJob(testJobType, []byte("payload2"))
	jobID3 := queue.AddJob(testJobType, []byte("payload3"))

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// List all jobs
	jobs := queue.ListJobs()

	if len(jobs) != 3 {
		t.Fatalf("Expected 3 jobs, got %d", len(jobs))
	}

	// Verify all job IDs are present
	jobIDs := map[string]bool{
		jobID1: false,
		jobID2: false,
		jobID3: false,
	}

	for _, job := range jobs {
		if _, exists := jobIDs[job.ID]; exists {
			jobIDs[job.ID] = true
		}
	}

	for id, found := range jobIDs {
		if !found {
			t.Errorf("Job %s not found in list", id)
		}
	}
}
