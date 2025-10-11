package kvstore

import (
	"sync"
)

// MemoryStore implements in-memory key-value storage
type MemoryStore struct {
	mu    sync.RWMutex
	data  map[string][]byte
	nodes []string // List of other nodes in the cluster
}

// NewMemoryStore creates a new instance of MemoryStore
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data:  make(map[string][]byte),
		nodes: make([]string, 0),
	}
}

// Set stores a value for a key
func (s *MemoryStore) Set(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = value
	return nil
}

// Get retrieves a value by key
func (s *MemoryStore) Get(key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, exists := s.data[key]
	return val, exists, nil
}

// Delete removes a key from the store
func (s *MemoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, key)
	return nil
}

// Close implements Store interface
func (s *MemoryStore) Close() error {
	return nil
}

// ForEach iterates over all key-value pairs in the store
func (s *MemoryStore) ForEach(fn func(key string, value []byte) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for k, v := range s.data {
		if err := fn(k, v); err != nil {
			return err
		}
	}
	return nil
}
