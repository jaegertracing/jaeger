package v1adapter

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
)

type DependencyReader struct {
	reader dependencystore.Reader
}

func GetV1DependencyReader(reader depstore.Reader) dependencystore.Reader {
	if dr, ok := reader.(*DependencyReader); ok {
		return dr.reader
	}
	return &DowngradedDependencyReader{
		reader: reader,
	}
}

func NewDependencyReader(reader dependencystore.Reader) *DependencyReader {
	return &DependencyReader{
		reader: reader,
	}
}

func (dr *DependencyReader) GetDependencies(
	ctx context.Context,
	query depstore.QueryParameters,
) ([]model.DependencyLink, error) {
	return dr.reader.GetDependencies(ctx, query.EndTime, query.EndTime.Sub(query.StartTime))
}

type DowngradedDependencyReader struct {
	reader depstore.Reader
}

func (dr *DowngradedDependencyReader) GetDependencies(
	ctx context.Context,
	endTs time.Time,
	lookback time.Duration,
) ([]model.DependencyLink, error) {
	return dr.reader.GetDependencies(ctx, depstore.QueryParameters{
		StartTime: endTs.Add(-lookback),
		EndTime:   endTs,
	})
}
