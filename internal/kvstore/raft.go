package kvstore

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"

	"github.com/Jeanedlune/archio/internal/metrics"
)

// Command represents an operation to be performed on the store
type Command struct {
	Op    string `json:"op"` // "set" or "delete"
	Key   string `json:"key"`
	Value []byte `json:"value,omitempty"`
}

// RaftNode manages the Raft consensus protocol
type RaftNode struct {
	raft     *raft.Raft
	store    Store
	dataDir  string
	bindAddr string
}

// NewRaftNode creates a new Raft node
func NewRaftNode(store Store, dataDir, bindAddr string, bootstrap bool) (*RaftNode, error) {
	node := &RaftNode{
		store:    store,
		dataDir:  dataDir,
		bindAddr: bindAddr,
	}

	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(bindAddr)

	// Setup Raft communication
	addr, err := net.ResolveTCPAddr("tcp", bindAddr)
	if err != nil {
		return nil, err
	}

	transport, err := raft.NewTCPTransport(bindAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, err
	}

	// Create the snapshot store
	snapshots, err := raft.NewFileSnapshotStore(dataDir, 2, os.Stderr)
	if err != nil {
		return nil, err
	}

	// Create the log store and stable store
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft-log.db"))
	if err != nil {
		return nil, err
	}

	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft-stable.db"))
	if err != nil {
		return nil, err
	}

	// Instantiate the Raft system
	ra, err := raft.NewRaft(config, (*FSM)(node), logStore, stableStore, snapshots, transport)
	if err != nil {
		return nil, err
	}
	node.raft = ra

	if bootstrap {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      config.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		ra.BootstrapCluster(configuration)
	}

	return node, nil
}

// FSM implements the raft.FSM interface
type FSM RaftNode

// Apply applies a Raft log entry to the key-value store
func (f *FSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return err
	}

	switch cmd.Op {
	case "set":
		return f.store.Set(cmd.Key, cmd.Value)
	case "delete":
		return f.store.Delete(cmd.Key)
	default:
		return fmt.Errorf("unknown command: %s", cmd.Op)
	}
}

// Snapshot returns a snapshot of the key-value store
func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	start := time.Now()
	phase := "snapshot"
	metrics.SnapshotOperations.WithLabelValues(phase, "started").Inc()

	snapshot := make(map[string][]byte)
	err := f.store.ForEach(func(key string, value []byte) error {
		snapshot[key] = value
		return nil
	})
	if err != nil {
		metrics.SnapshotErrors.WithLabelValues(phase).Inc()
		metrics.SnapshotOperations.WithLabelValues(phase, "failed").Inc()
		return nil, err
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		metrics.SnapshotErrors.WithLabelValues(phase).Inc()
		metrics.SnapshotOperations.WithLabelValues(phase, "failed").Inc()
		return nil, err
	}

	// Observe duration/size
	metrics.SnapshotDuration.Observe(time.Since(start).Seconds())
	metrics.SnapshotSizeBytes.Observe(float64(len(data)))
	metrics.SnapshotOperations.WithLabelValues(phase, "succeeded").Inc()

	return &fsmSnapshot{data: data}, nil
}

// Restore restores the key-value store from a snapshot
func (f *FSM) Restore(rc io.ReadCloser) error {
	start := time.Now()
	phase := "restore"
	metrics.SnapshotOperations.WithLabelValues(phase, "started").Inc()
	defer rc.Close()

	var snapshot map[string][]byte
	if err := json.NewDecoder(rc).Decode(&snapshot); err != nil {
		metrics.SnapshotErrors.WithLabelValues(phase).Inc()
		metrics.SnapshotOperations.WithLabelValues(phase, "failed").Inc()
		return err
	}

	// Clear the current store by iterating and deleting
	err := f.store.ForEach(func(key string, _ []byte) error {
		return f.store.Delete(key)
	})
	if err != nil {
		metrics.SnapshotErrors.WithLabelValues(phase).Inc()
		metrics.SnapshotOperations.WithLabelValues(phase, "failed").Inc()
		return err
	}

	// Restore from snapshot
	for key, value := range snapshot {
		if err := f.store.Set(key, value); err != nil {
			metrics.SnapshotErrors.WithLabelValues(phase).Inc()
			metrics.SnapshotOperations.WithLabelValues(phase, "failed").Inc()
			return err
		}
	}

	metrics.SnapshotRestoreDuration.Observe(time.Since(start).Seconds())
	metrics.SnapshotOperations.WithLabelValues(phase, "succeeded").Inc()

	return nil
}

// fsmSnapshot implements the raft.FSMSnapshot interface
type fsmSnapshot struct {
	data []byte
}

func (f *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	_, err := sink.Write(f.data)
	if err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

func (f *fsmSnapshot) Release() {
	// Nothing to release as our snapshot data is just a byte slice
}
