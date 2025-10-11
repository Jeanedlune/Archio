package kvstore

import (
	"os"
	"testing"
)

// fileSnapshotSink implements raft.SnapshotSink by writing snapshot bytes to a file
type fileSnapshotSink struct {
	f *os.File
	p string
}

func newFileSnapshotSink(path string) (*fileSnapshotSink, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &fileSnapshotSink{f: f, p: path}, nil
}

func (s *fileSnapshotSink) ID() string { return s.p }
func (s *fileSnapshotSink) Cancel() error {
	_ = s.f.Close()
	_ = os.Remove(s.p)
	return nil
}
func (s *fileSnapshotSink) Close() error { return s.f.Close() }

func (s *fileSnapshotSink) Write(p []byte) (int, error) {
	return s.f.Write(p)
}

func TestSnapshotPersistToFile(t *testing.T) {
	store := NewMemoryStore()
	fsm := &FSM{store: store}

	// Put some data
	if err := store.Set("alpha", []byte("one")); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if err := store.Set("beta", []byte("two")); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	snapshot, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	// Persist to temp file
	f, err := os.CreateTemp("", "snapshot-*.bin")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := f.Name()
	_ = f.Close()

	sink, err := newFileSnapshotSink(path)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}

	if err := snapshot.Persist(sink); err != nil {
		t.Fatalf("persist: %v", err)
	}

	// Now restore from file into new store
	b, err := os.Open(path)
	if err != nil {
		t.Fatalf("open snapshot file: %v", err)
	}
	defer func() {
		_ = b.Close()
	}()

	store2 := NewMemoryStore()
	fsm2 := &FSM{store: store2}
	if err := fsm2.Restore(b); err != nil {
		t.Fatalf("restore from file: %v", err)
	}

	// Validate
	v, ok, err := store2.Get("alpha")
	if err != nil || !ok || string(v) != "one" {
		t.Fatalf("alpha mismatch: %v %v %s", err, ok, string(v))
	}
	v, ok, err = store2.Get("beta")
	if err != nil || !ok || string(v) != "two" {
		t.Fatalf("beta mismatch: %v %v %s", err, ok, string(v))
	}

	// cleanup
	_ = os.Remove(path)
}
