package kvstore

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/raft"
)

// helper to start a single-node raft, apply data and return dataDir and node
func startSingleNodeRaftWithData(t *testing.T, data map[string][]byte) (string, *RaftNode) {
	dataDir, err := os.MkdirTemp("", "raft-int-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}

	store := NewMemoryStore()
	node, err := NewRaftNode(store, dataDir, "127.0.0.1:0", true)
	if err != nil {
		_ = os.RemoveAll(dataDir)
		t.Fatalf("NewRaftNode: %v", err)
	}
	if err := waitForLeader(node, 5*time.Second); err != nil {
		node.raft.Shutdown()
		_ = os.RemoveAll(dataDir)
		t.Fatalf("leader wait: %v", err)
	}
	for k, v := range data {
		cmd := Command{Op: "set", Key: k, Value: v}
		b, _ := json.Marshal(cmd)
		f := node.raft.Apply(b, 2*time.Second)
		if err := f.Error(); err != nil {
			node.raft.Shutdown()
			_ = os.RemoveAll(dataDir)
			t.Fatalf("apply error: %v", err)
		}
	}
	return dataDir, node
}

func TestRaftSnapshotFileUsable_StartAndSnapshot(t *testing.T) {
	t.Skip("Skipping due to raft-boltdb compatibility issue")
	testData := map[string][]byte{"r1": []byte("v1"), "r2": []byte("v2")}
	dataDir, node := startSingleNodeRaftWithData(t, testData)
	defer func() { node.raft.Shutdown(); os.RemoveAll(dataDir) }()

	// Create a snapshot from the node's FSM directly and persist to file
	fsm := (*FSM)(node)
	snapshot, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("fsm.Snapshot failed: %v", err)
	}

	// persist snapshot to temp file using file-backed sink
	tf, err := os.CreateTemp("", "raft-fsm-snap-*.bin")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := tf.Name()
	_ = tf.Close()
	sink, err := newFileSnapshotSink(path)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	if err := snapshot.Persist(sink); err != nil {
		t.Fatalf("persist fsm snapshot: %v", err)
	}

	// open snapshot file and restore into a fresh store
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	newStore := NewMemoryStore()
	newFSM := &FSM{store: newStore}
	if err := newFSM.Restore(f); err != nil {
		t.Fatalf("restore from fsm snapshot file failed: %v", err)
	}

	for k, want := range testData {
		got, ok, err := newStore.Get(k)
		if err != nil || !ok || string(got) != string(want) {
			t.Fatalf("restored key %s mismatch: got=%s ok=%v err=%v", k, string(got), ok, err)
		}
	}
}

// waitForLeader waits until the given raft node becomes leader or times out.
func waitForLeader(node *RaftNode, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if node.raft.State() == raft.Leader {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("node did not become leader within %v", timeout)
}

