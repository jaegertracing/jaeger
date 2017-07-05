package dependencystore

import (
	"time"

	"github.com/uber/jaeger/model"
)

type DependencyStore struct {}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore() *DependencyStore {
	return &DependencyStore{}
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	return nil
}

// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return nil, nil
}