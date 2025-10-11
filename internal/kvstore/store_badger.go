package kvstore

import (
	"github.com/dgraph-io/badger/v4"
)

// BadgerStore implements persistent storage using BadgerDB
type BadgerStore struct {
	db *badger.DB
}

// NewBadgerStore creates a new BadgerDB-backed store
func NewBadgerStore(path string) (*BadgerStore, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil // Disable Badger's default logger

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	return &BadgerStore{
		db: db,
	}, nil
}

// Set stores a value for a key
func (s *BadgerStore) Set(key string, value []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), value)
	})
}

// Get retrieves a value by key
func (s *BadgerStore) Get(key string) ([]byte, bool, error) {
	var value []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}

		value, err = item.ValueCopy(nil)
		return err
	})

	if err != nil {
		return nil, false, err
	}

	return value, value != nil, nil
}

// Delete removes a key from the store
func (s *BadgerStore) Delete(key string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

// Close closes the database
func (s *BadgerStore) Close() error {
	return s.db.Close()
}

// ForEach iterates over all key-value pairs in the store
func (s *BadgerStore) ForEach(fn func(key string, value []byte) error) error {
	return s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			err := item.Value(func(v []byte) error {
				return fn(string(k), v)
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}
