package kvstore

// Store defines the interface for key-value storage
type Store interface {
	Set(key string, value []byte) error
	Get(key string) ([]byte, bool, error)
	Delete(key string) error
	Close() error
	// ForEach iterates over all key-value pairs in the store
	ForEach(fn func(key string, value []byte) error) error
}
