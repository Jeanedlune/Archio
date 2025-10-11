package kvstore

import (
	"bytes"
	"io"
	"testing"
)

// mockSnapshotSink implements raft.SnapshotSink for testing
type mockSnapshotSink struct {
	*bytes.Buffer
	cancel bool
}

func NewMockSnapshotSink() *mockSnapshotSink {
	return &mockSnapshotSink{Buffer: &bytes.Buffer{}}
}

func (m *mockSnapshotSink) ID() string    { return "mock" }
func (m *mockSnapshotSink) Cancel() error { m.cancel = true; return nil }
func (m *mockSnapshotSink) Close() error  { return nil }

// Write implements io.Writer for the snapshot sink. If the underlying
// buffer is nil we return an error instead of causing a panic. This
// allows tests to simulate write failures safely.
func (m *mockSnapshotSink) Write(p []byte) (int, error) {
	if m.Buffer == nil {
		return 0, io.ErrClosedPipe
	}
	return m.Buffer.Write(p)
}

// mockReadCloser wraps a bytes.Reader to implement io.ReadCloser
type mockReadCloser struct {
	*bytes.Reader
}

func NewMockReadCloser(data []byte) io.ReadCloser {
	return &mockReadCloser{bytes.NewReader(data)}
}

func (m *mockReadCloser) Close() error { return nil }

func TestSnapshotRestore(t *testing.T) {
	// Create a store and FSM
	store := NewMemoryStore()
	fsm := &FSM{store: store}

	// Add test data
	testData := map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
		"key3": []byte("value3"),
	}

	for k, v := range testData {
		if err := store.Set(k, v); err != nil {
			t.Fatalf("Failed to set initial value: %v", err)
		}
	}

	// Take a snapshot
	snapshot, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	// Create a sink and persist the snapshot
	sink := NewMockSnapshotSink()
	if err := snapshot.Persist(sink); err != nil {
		t.Fatalf("Failed to persist snapshot: %v", err)
	}

	// Create a new store and FSM
	store2 := NewMemoryStore()
	fsm2 := &FSM{store: store2}

	// Restore from snapshot using our mock ReadCloser
	rc := NewMockReadCloser(sink.Bytes())
	if err := fsm2.Restore(rc); err != nil {
		t.Fatalf("Failed to restore from snapshot: %v", err)
	}

	// Verify restored data
	for k, want := range testData {
		got, exists, err := store2.Get(k)
		if err != nil {
			t.Fatalf("Failed to get restored value: %v", err)
		}
		if !exists {
			t.Errorf("Key %s missing after restore", k)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("Key %s: got %q, want %q", k, got, want)
		}
	}

	// Test snapshot error cases
	t.Run("Failed persist", func(t *testing.T) {
		sink := NewMockSnapshotSink()
		sink.Buffer = nil // Force write error
		if err := snapshot.Persist(sink); err == nil {
			t.Error("Expected error when persisting to nil buffer")
		}
	})
}
