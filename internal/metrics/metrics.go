package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// KVStore metrics
	KVOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kvstore_operations_total",
		Help: "The total number of KV store operations",
	}, []string{"operation", "status"})

	KVOperationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "kvstore_operation_duration_seconds",
		Help:    "Duration of KV store operations",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation"})

	// Job Queue metrics
	JobsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "jobs_processed_total",
		Help: "The total number of processed jobs",
	}, []string{"type", "status"})

	JobProcessingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "job_processing_duration_seconds",
		Help:    "Duration of job processing",
		Buckets: prometheus.DefBuckets,
	}, []string{"type"})

	JobsInQueue = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "jobs_in_queue",
		Help: "The current number of jobs in the queue",
	})

	WorkerCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "worker_count",
		Help: "The current number of active workers",
	})

	// Snapshot / restore metrics for KV store
	SnapshotOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kvstore_snapshot_operations_total",
		Help: "The total number of snapshot operations",
	}, []string{"phase", "status"})

	SnapshotDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "kvstore_snapshot_duration_seconds",
		Help:    "Duration of snapshot creation (seconds)",
		Buckets: prometheus.DefBuckets,
	})

	SnapshotSizeBytes = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "kvstore_snapshot_size_bytes",
		Help: "Size of snapshots in bytes",
		// reasonable exponential buckets for snapshot sizes
		Buckets: prometheus.ExponentialBuckets(256, 2, 10),
	})

	SnapshotRestoreDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "kvstore_snapshot_restore_duration_seconds",
		Help:    "Duration to restore a snapshot (seconds)",
		Buckets: prometheus.DefBuckets,
	})

	SnapshotErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kvstore_snapshot_errors_total",
		Help: "The total number of errors encountered during snapshot or restore",
	}, []string{"phase"})
)
