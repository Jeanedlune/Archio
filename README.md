# Archio - Distributed Key-Value Store with Job Queue

This project implements a distributed key-value store with an integrated job queue system in Go.

## Features

### Key-Value Store
- In-memory storage with persistent backup
- Distributed consensus for consistency (Hashicorp Raft)
- Snapshot/restore support for persisting FSM state
- REST API interface

### Job Queue
- Worker pool management
- Job scheduling and priority
- Persistent job storage
- Retry mechanism
- Job status monitoring

## Getting Started

1. Clone the repository
2. Install dependencies:
   ```bash
   go mod tidy
   ```
3. Run the server:
   ```bash
   go run cmd/server/main.go
   ```

## Project Structure

```
.
├── cmd/
│   └── server/       # Main application entrypoints
├── internal/
│   ├── kvstore/     # Key-value store implementation
│   └── jobqueue/    # Job queue implementation
├── pkg/
│   └── api/         # Public API definitions
└── configs/         # Configuration files
```

## Configuration

TBD - Configuration options will be documented here.

## Snapshot & Restore

- The KV store supports creating and restoring point-in-time snapshots. Snapshots are used by the Raft FSM to persist the full key/value state. Current snapshot format: JSON map of key -> bytes.
- Snapshot creation is implemented using `Store.ForEach` to collect all key/value pairs and is persisted through Raft snapshot sinks.

Notes:
- Snapshot creation and restore are instrumented with Prometheus metrics (see Metrics below).
- Restores currently clear the target store and then re-populate it from the snapshot. For production use consider implementing atomic swap semantics.

## API Documentation

TBD - API endpoints will be documented here.

## Metrics

- The project exposes Prometheus metrics for core subsystems under the `internal/metrics` package. Important metrics added recently:
  - `kvstore_snapshot_operations_total{phase, status}` — snapshot/restore operation counts (phase: "snapshot"|"persist"|"restore", status: "started"|"succeeded"|"failed").
  - `kvstore_snapshot_duration_seconds` — histogram of snapshot creation durations.
  - `kvstore_snapshot_size_bytes` — histogram of snapshot sizes in bytes.
  - `kvstore_snapshot_restore_duration_seconds` — histogram of restore durations.

To scrape metrics, ensure the HTTP server exposes the Prometheus handler (example: `/metrics`) and add a scrape target in your Prometheus scrape config.

## Testing

- Unit and integration tests live in the `internal` packages (for example `internal/kvstore`).
- There are tests for snapshot persist/restore (file-backed and in-memory) and a single-node Raft integration test.

Run tests:

```bash
# all tests
go test ./...

# only kvstore tests
go test ./internal/kvstore -v
```

## Contributing

Feel free to submit issues and enhancement requests!

