package kvstore

import (
	"encoding/json"

	"time"

	"github.com/hashicorp/raft"

	"github.com/Jeanedlune/archio/internal/metrics"
)

// StoreSnapshot represents a point-in-time snapshot of the key-value store
type StoreSnapshot struct {
	store Store
}

// Persist writes the snapshot to the given sink
func (s *StoreSnapshot) Persist(sink raft.SnapshotSink) error {
	start := time.Now()
	phase := "persist"
	metrics.SnapshotOperations.WithLabelValues(phase, "started").Inc()
	// Get all keys and values from the store
	snapshot := make(map[string][]byte)
	err := s.store.ForEach(func(key string, value []byte) error {
		snapshot[key] = value
		return nil
	})
	if err != nil {
		metrics.SnapshotErrors.WithLabelValues(phase).Inc()
		metrics.SnapshotOperations.WithLabelValues(phase, "failed").Inc()
		sink.Cancel()
		return err
	}

	// Serialize the snapshot
	data, err := json.Marshal(snapshot)
	if err != nil {
		metrics.SnapshotErrors.WithLabelValues(phase).Inc()
		metrics.SnapshotOperations.WithLabelValues(phase, "failed").Inc()
		sink.Cancel()
		return err
	}

	// Write the snapshot
	if _, err := sink.Write(data); err != nil {
		metrics.SnapshotErrors.WithLabelValues(phase).Inc()
		metrics.SnapshotOperations.WithLabelValues(phase, "failed").Inc()
		sink.Cancel()
		return err
	}

	// observe metrics
	duration := time.Since(start).Seconds()
	metrics.SnapshotDuration.Observe(duration)
	metrics.SnapshotSizeBytes.Observe(float64(len(data)))
	metrics.SnapshotOperations.WithLabelValues(phase, "succeeded").Inc()

	return sink.Close()
}

// Release is called when we are finished with the snapshot
func (s *StoreSnapshot) Release() {
	// No resources to release
}
