package depstore

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/model"
)

// Reader can load service dependencies from storage.
type Reader interface {
	GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error)
}
