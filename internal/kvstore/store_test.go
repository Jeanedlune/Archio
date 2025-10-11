package kvstore

import (
	"os"
	"testing"
)

const (
	testKey          = "test-key"
	testValue        = "test-value"
	keyShouldExist   = "Key should exist"
	errSetValue      = "Failed to set value: %v"
	errGetValue      = "Failed to get value: %v"
	errDeleteKey     = "Failed to delete key: %v"
	errCreateTempDir = "Failed to create temp dir: %v"
	errCreateStore   = "Failed to create BadgerStore: %v"
)

// Helper functions
func assertNoError(t *testing.T, err error, format string) {
	t.Helper()
	if err != nil {
		t.Fatalf(format, err)
	}
}

func assertKeyExists(t *testing.T, exists bool, shouldExist bool) {
	t.Helper()
	if exists != shouldExist {
		if shouldExist {
			t.Fatal(keyShouldExist)
		} else {
			t.Error("Key should not exist after deletion")
		}
	}
}

func testStoreOperations(t *testing.T, store Store) {
	t.Helper()

	t.Run("Set and Get", func(t *testing.T) {
		err := store.Set(testKey, []byte(testValue))
		assertNoError(t, err, errSetValue)

		got, exists, err := store.Get(testKey)
		assertNoError(t, err, errGetValue)
		assertKeyExists(t, exists, true)

		if string(got) != testValue {
			t.Errorf("Got %s, want %s", string(got), testValue)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := store.Set(testKey, []byte(testValue))
		assertNoError(t, err, errSetValue)

		err = store.Delete(testKey)
		assertNoError(t, err, errDeleteKey)

		_, exists, err := store.Get(testKey)
		assertNoError(t, err, errGetValue)
		assertKeyExists(t, exists, false)
	})
}

func TestMemoryStore(t *testing.T) {
	store := NewMemoryStore()
	testStoreOperations(t, store)
}

func TestBadgerStore(t *testing.T) {
	dir, err := os.MkdirTemp("", "badger-test")
	assertNoError(t, err, errCreateTempDir)
	defer os.RemoveAll(dir)

	store, err := NewBadgerStore(dir)
	assertNoError(t, err, errCreateStore)
	defer func() {
		_ = store.Close()
	}()

	testStoreOperations(t, store)
}
