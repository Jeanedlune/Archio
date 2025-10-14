# Job Persistence Implementation

## Overview

This document describes the job persistence feature that allows jobs to survive application restarts.

## Implementation Details

### Architecture

The job persistence feature uses the existing KV store infrastructure to persist job state:

1. **Storage Interface**: Jobs are stored using the `Store` interface that's already used for KV operations
2. **Key Prefix**: All job keys use the prefix `job:` followed by the job ID
3. **Serialization**: Jobs are serialized to JSON for storage
4. **Recovery**: On startup, jobs are automatically recovered from persistent storage

### Key Components

#### Queue Modifications

The `Queue` struct now includes:
- `store Store`: Optional persistent storage backend
- `saveJob()`: Persists job state changes
- `recoverJobs()`: Recovers jobs on startup
- `deleteJob()`: Removes jobs from storage

#### Job Lifecycle with Persistence

1. **Job Creation**: When a job is added via `AddJob()`, it's immediately persisted
2. **State Changes**: Every status change (pending → processing → completed/failed) is persisted
3. **Recovery**: On startup, all jobs are loaded from storage
4. **Processing Jobs**: Jobs in "processing" state are reset to "pending" to avoid duplicate processing

### API Changes

#### New Functions

```go
// Create queue with persistence
queue := jobqueue.NewQueueWithStore(store)

// Create queue with custom config and persistence
queue := jobqueue.NewQueueWithConfig(config, store)

// Graceful shutdown
queue.Shutdown()
```

#### Backward Compatibility

Existing code continues to work without changes:
```go
// Works without persistence
queue := jobqueue.NewQueue()
```

### Configuration

Job persistence is automatically enabled when the queue is initialized with a store:

```go
// In main.go
store := kvstore.NewBadgerStore(dataDir)
queue := jobqueue.NewQueueWithConfig(queueConfig, store)
```

### Recovery Behavior

On application restart:

1. **Completed Jobs**: Remain in completed state, not re-queued
2. **Failed Jobs**: Remain in failed state, not re-queued
3. **Pending Jobs**: Re-queued for processing
4. **Processing Jobs**: Reset to pending and re-queued (prevents duplicate processing)

### Testing

Comprehensive tests are provided in `persistence_test.go`:

- `TestJobPersistence`: Verifies jobs are persisted
- `TestJobRecovery`: Tests job recovery after restart
- `TestJobRecoveryNoDuplicateProcessing`: Ensures no duplicate processing
- `TestJobRecoveryProcessingToPending`: Verifies processing jobs are reset
- `TestJobPersistenceStateChanges`: Tests all state changes are persisted
- `TestBackwardCompatibility`: Ensures existing code works

## Usage Example

```go
package main

import (
    "context"
    "fmt"
    "github.com/Jeanedlune/archio/internal/jobqueue"
    "github.com/Jeanedlune/archio/internal/kvstore"
)

// EmailHandler is an example handler for email jobs
type EmailHandler struct{}

func (h *EmailHandler) Handle(ctx context.Context, job *jobqueue.Job) error {
    fmt.Printf("Sending email with payload: %s\n", string(job.Payload))
    // Implement actual email sending logic here
    return nil
}

func main() {
    // Initialize persistent storage
    store, err := kvstore.NewBadgerStore("./data/jobs")
    if err != nil {
        panic(err)
    }
    defer store.Close()
    
    // Create queue with persistence
    queue := jobqueue.NewQueueWithStore(store)
    defer queue.Shutdown()
    
    // Register handlers
    queue.RegisterHandler("email", &EmailHandler{})
    
    // Add jobs - they will survive restarts
    jobID := queue.AddJob("email", []byte(`{"to": "user@example.com"}`))
    fmt.Printf("Created job: %s\n", jobID)
    
    // Jobs are automatically recovered on next startup
}
```

## Benefits

1. **Reliability**: Jobs survive application crashes and restarts
2. **Durability**: Job state and metadata are preserved
3. **No Duplicate Processing**: Processing jobs are properly handled on restart
4. **Backward Compatible**: Existing code works without changes
5. **Flexible**: Can run with or without persistence

## Performance Considerations

- Job state is persisted synchronously on each state change
  - Current implementation prioritizes data consistency over throughput
  - For high-throughput scenarios, consider implementing async persistence
- Uses the same BadgerDB backend as KV store (efficient)
- Minimal overhead for in-memory operations
- Recovery time depends on number of stored jobs
- Lock is held during persistence operations to ensure consistency

## Future Enhancements

Potential improvements:
- Batch persistence for better performance
- Job expiration/cleanup for old completed jobs
- Job history and audit trail
- Priority-based job recovery
- Distributed job queue with Raft consensus
