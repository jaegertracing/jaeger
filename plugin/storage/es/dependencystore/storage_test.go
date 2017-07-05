package dependencystore

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"time"
)

type depStorageTest struct {
	storage   *DependencyStore
}

func withDepStore(fn func(s *depStorageTest)) {
	s := &depStorageTest{
		storage:   NewDependencyStore(),
	}
	fn(s)
}

func TestNewDependencyStore(t *testing.T) {
	withDepStore(func(s *depStorageTest) {
		assert.NotNil(t, s)
	})
}

func TestDependencyStore_WriteDependencies(t *testing.T) {
	withDepStore(func(s *depStorageTest) {
		assert.NoError(t, s.storage.WriteDependencies(time.Time{}, nil))
	})
}

func TestDependencyStore_GetDependencies(t *testing.T) {
	withDepStore(func(s *depStorageTest) {
		result, err := s.storage.GetDependencies(time.Time{}, time.Duration(0))
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
}
