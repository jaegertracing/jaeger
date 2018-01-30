package dependencystore

import (
  "fmt"

  "context"
  "time"

  "github.com/pkg/errors"
  "go.uber.org/zap"

  "github.com/jaegertracing/jaeger/model"

  agentReporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
)

// DependencyStore handles all queries and insertions of dependencies
type DependencyStore struct {
  ctx    context.Context
  client *agentReporter.Reporter
  logger *zap.Logger
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(client *agentReporter.Reporter, logger *zap.Logger) *DependencyStore {
  return &DependencyStore{
    ctx:    context.Background(),
    client: client,
    logger: logger,
  }
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *DependencyStore) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
  s.logger.Info(fmt.Sprintf("%+v", dependencies))
  return nil
}


// GetDependencies returns all interservice dependencies
func (s *DependencyStore) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
  return nil, errors.New("Reading not supported.")
}