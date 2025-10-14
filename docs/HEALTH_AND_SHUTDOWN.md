# Health Checks and Graceful Shutdown

## Overview

This document describes the health check endpoints and graceful shutdown behavior implemented for production deployments and container orchestration platforms.

## Health Check Endpoints

### `/health` - Liveness Probe

Basic health check that indicates the application is running.

**Response:**
- **200 OK**: Application is alive
- Returns: `{"status": "ok"}`

**Use Case:**
- Kubernetes liveness probe
- Load balancer health checks
- Basic monitoring

**Example:**
```bash
curl http://localhost:8080/health
```

### `/ready` - Readiness Probe

Readiness check that indicates the application is ready to accept traffic.

**Response:**
- **200 OK**: Application is ready
  - Returns: `{"status": "ready"}`
- **503 Service Unavailable**: Application is not ready
  - Returns: `{"status": "not ready"}`

**Use Case:**
- Kubernetes readiness probe
- Load balancer traffic routing
- Rolling deployments

**Example:**
```bash
curl http://localhost:8080/ready
```

## Readiness States

The server transitions through the following readiness states:

1. **Startup**: Not ready (default state)
2. **Running**: Ready (after successful initialization)
3. **Shutting Down**: Not ready (during graceful shutdown)

## Graceful Shutdown

The application implements a comprehensive graceful shutdown process to ensure:
- No abrupt connection termination
- Proper resource cleanup
- No data loss
- Clean process exit

### Shutdown Sequence

When receiving SIGINT or SIGTERM signal:

1. **Mark as Not Ready** (immediate)
   - Server stops accepting new connections
   - Load balancers stop routing traffic
   - Readiness probe returns 503

2. **HTTP Server Shutdown** (with timeout)
   - Stops accepting new connections
   - Waits for in-flight requests to complete
   - Timeout: 30 seconds

3. **Raft Node Shutdown** (if enabled)
   - Gracefully leaves the cluster
   - Closes Raft connections

4. **Job Queue Shutdown**
   - Stops accepting new jobs
   - Cancels worker goroutines
   - In-flight jobs may be interrupted

5. **Storage Cleanup**
   - Flushes pending writes
   - Closes database connections
   - Releases file locks

### Shutdown Timeout

Default shutdown timeout: **30 seconds**

If shutdown doesn't complete within timeout:
- Remaining operations are cancelled
- Resources may not be fully cleaned up
- Process exits with logged errors

### Signal Handling

Supported signals:
- **SIGINT** (Ctrl+C): Graceful shutdown
- **SIGTERM**: Graceful shutdown (default for Docker/K8s)

## Container Orchestration

### Kubernetes Configuration

#### Liveness Probe
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
```

#### Readiness Probe
```yaml
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 2
```

#### Graceful Shutdown
```yaml
spec:
  terminationGracePeriodSeconds: 45  # Should be > shutdown timeout
  containers:
  - name: archio
    lifecycle:
      preStop:
        exec:
          command: ["/bin/sh", "-c", "sleep 5"]  # Allow time for readiness to propagate
```

### Docker Compose

```yaml
services:
  archio:
    image: archio:latest
    ports:
      - "8080:8080"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 10s
    stop_grace_period: 45s
```

### Docker

```bash
# Run with health check
docker run -d \
  --name archio \
  --health-cmd="curl -f http://localhost:8080/health || exit 1" \
  --health-interval=10s \
  --health-timeout=5s \
  --health-retries=3 \
  --health-start-period=10s \
  -p 8080:8080 \
  archio:latest

# Graceful shutdown
docker stop --time=45 archio
```

## Monitoring

### Health Check Monitoring

Monitor health check endpoints to detect issues:

```bash
# Continuous monitoring
watch -n 5 'curl -s http://localhost:8080/health && curl -s http://localhost:8080/ready'
```

### Shutdown Logs

During graceful shutdown, the application logs each step:

```
Received shutdown signal, starting graceful shutdown...
Server marked as not ready
Shutting down HTTP server...
HTTP server shut down successfully
Shutting down Raft node...
Raft node shut down successfully
Shutting down job queue...
Job queue shut down successfully
Closing storage...
Storage closed successfully
Graceful shutdown completed
```

Monitor these logs to ensure clean shutdowns.

## Best Practices

### Load Balancers

1. Configure health checks with appropriate intervals
2. Use `/ready` for traffic routing decisions
3. Use `/health` for instance health monitoring
4. Set proper timeout values (3-5 seconds)

### Rolling Deployments

1. Wait for readiness before routing traffic
2. Ensure old instances complete gracefully
3. Set `terminationGracePeriodSeconds` > shutdown timeout
4. Use `preStop` hook to allow readiness propagation

### Development

```bash
# Start server
go run cmd/server/main.go

# Test health checks
curl http://localhost:8080/health
curl http://localhost:8080/ready

# Graceful shutdown (Ctrl+C or)
kill -SIGTERM <pid>
```

### Production

1. Always use readiness probes in production
2. Monitor shutdown logs for errors
3. Set appropriate timeouts based on workload
4. Test graceful shutdown in staging

## Troubleshooting

### Server Not Ready

If `/ready` returns 503:
- Check if server finished initialization
- Check logs for startup errors
- Verify all dependencies are available

### Shutdown Timeout

If shutdown exceeds timeout:
- Increase shutdown timeout in code
- Increase `terminationGracePeriodSeconds` in K8s
- Check for stuck goroutines or connections
- Review shutdown logs for bottlenecks

### Resource Leaks

If resources aren't cleaned up:
- Check shutdown logs for errors
- Verify all Close() calls succeed
- Monitor file descriptors and connections
- Use profiling tools to detect leaks

## Future Enhancements

Potential improvements:
- Deep health checks (database connectivity, disk space)
- Configurable shutdown timeout
- Graceful job completion before shutdown
- Health check metrics and history
- Startup probes for slow-starting applications
