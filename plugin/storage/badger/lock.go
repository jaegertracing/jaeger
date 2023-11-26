package badger

import "time"

type lock struct{}

// Acquire always returns true for badgerdb as no lock is needed
func (l *lock) Acquire(resource string, ttl time.Duration) (bool, error) {
	return true, nil
}

// Forfeit always returns true for badgerdb as no lock is needed
func (l *lock) Forfeit(resource string) (bool, error) {
	return true, nil
}
