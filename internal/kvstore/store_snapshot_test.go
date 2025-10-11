package kvstore

import (
	"testing"
)

func TestStoreSnapshot(t *testing.T) {
	store := NewMemoryStore()

	// Add some test data
	testData := map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
		"key3": []byte("value3"),
	}

	for k, v := range testData {
		if err := store.Set(k, v); err != nil {
			t.Fatalf("Failed to set value: %v", err)
		}
	}

	// Verify data was added
	for k, v := range testData {
		got, exists, err := store.Get(k)
		if err != nil {
			t.Fatalf("Failed to get value: %v", err)
		}
		if !exists {
			t.Fatalf("Key %s missing", k)
		}
		if string(got) != string(v) {
			t.Errorf("Got %s, want %s", string(got), string(v))
		}
	}

	// Test ForEach
	count := 0
	err := store.ForEach(func(key string, value []byte) error {
		if _, ok := testData[key]; !ok {
			t.Errorf("Unexpected key: %s", key)
		}
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("ForEach failed: %v", err)
	}
	if count != len(testData) {
		t.Errorf("ForEach visited %d items, want %d", count, len(testData))
	}
}
