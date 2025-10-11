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
   Or with a configuration file:
   ```bash
   go run cmd/server/main.go -config config.yaml
   ```
   The server will start on http://localhost:8080 (or the configured port)

## Project Structure

```
.
├── cmd/
│   └── server/       # Main application entrypoints
├── internal/
│   ├── api/          # HTTP API handlers
│   ├── kvstore/      # Key-value store implementation
│   ├── jobqueue/     # Job queue implementation
│   └── metrics/      # Prometheus metrics
├── configs/          # Configuration files (TBD)
└── data/             # Data directory (created at runtime)
```

## Configuration

Archio supports configuration via YAML files. Copy `config.example.yaml` to `config.yaml` and modify as needed.

Run the server with a config file:
```bash
go run cmd/server/main.go -config config.yaml
```

If no config file is specified, default values are used.

### Configuration Options

- **server.port**: HTTP server port (default: ":8080")
- **storage.data_dir**: Base directory for data storage (default: "data")
- **storage.type**: Storage backend - "badger" for persistent or "memory" for in-memory (default: "badger")
- **raft.enabled**: Enable distributed consensus (default: false)
- **raft.bind_addr**: Raft communication address (default: ":8081")
- **raft.bootstrap**: Bootstrap the first cluster node (default: true)
- **job_queue.worker_count**: Number of job processing workers (default: 5)
- **job_queue.queue_size**: Maximum queued jobs (default: 100)
- **job_queue.max_retries**: Maximum retry attempts (default: 3)
- **job_queue.retry_backoff**: Retry backoff duration (default: "5s")

## Snapshot & Restore

- The KV store supports creating and restoring point-in-time snapshots. Snapshots are used by the Raft FSM to persist the full key/value state. Current snapshot format: JSON map of key -> bytes.
- Snapshot creation is implemented using `Store.ForEach` to collect all key/value pairs and is persisted through Raft snapshot sinks.

Notes:
- Snapshot creation and restore are instrumented with Prometheus metrics (see Metrics below).
- Restores currently clear the target store and then re-populate it from the snapshot. For production use consider implementing atomic swap semantics.

## API Documentation

The server exposes a REST API on port 8080. All requests and responses use JSON format.

### Key-Value Store API

- **POST /kv/{key}** - Set a value for a key
  - Request body: `{"value": "string"}`
  - Response: 200 OK

- **GET /kv/{key}** - Get the value for a key
  - Response: `{"value": "string"}` or 404 if key not found

### Job Queue API

- **POST /jobs/** - Create a new job
  - Request body: `{"type": "string", "payload": "string"}`
  - Response: `{"job_id": "string"}` (201 Created)

- **GET /jobs/{id}** - Get job details
  - Response: Job object or 404 if not found

### Metrics

- **GET /metrics** - Prometheus metrics endpoint

## API Usage Examples

### Set a key-value pair
```bash
curl -X POST http://localhost:8080/kv/mykey \
  -H "Content-Type: application/json" \
  -d '{"value": "Hello World"}'
```

### Get a value
```bash
curl http://localhost:8080/kv/mykey
# Response: {"value": "Hello World"}
```

### Create a job
```bash
curl -X POST http://localhost:8080/jobs/ \
  -H "Content-Type: application/json" \
  -d '{"type": "email", "payload": "user@example.com"}'
# Response: {"job_id": "job-123"}
```

### Get job status
```bash
curl http://localhost:8080/jobs/job-123
```

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

