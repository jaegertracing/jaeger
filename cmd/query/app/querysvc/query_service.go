// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package querysvc

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/pkg/multierror"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	errNoArchiveSpanStorage = errors.New("archive span storage was not configured")
)

const (
	defaultMaxClockSkewAdjust = time.Second
)

// QueryServiceOptions has optional members of QueryService
type QueryServiceOptions struct {
	ArchiveSpanReader spanstore.Reader
	ArchiveSpanWriter spanstore.Writer
	Adjuster          adjuster.Adjuster
}

// QueryService contains span utils required by the query-service.
type QueryService struct {
	spanReader       spanstore.Reader
	dependencyReader dependencystore.Reader
	options          QueryServiceOptions
}

// NewQueryService returns a new QueryService.
func NewQueryService(spanReader spanstore.Reader, dependencyReader dependencystore.Reader, options QueryServiceOptions) *QueryService {
	qsvc := &QueryService{
		spanReader:       spanReader,
		dependencyReader: dependencyReader,
		options:          options,
	}

	if qsvc.options.Adjuster == nil {
		qsvc.options.Adjuster = adjuster.Sequence(StandardAdjusters(defaultMaxClockSkewAdjust)...)
	}
	return qsvc
}

// GetTrace is the queryService implementation of spanstore.Reader.GetTrace
func (qs QueryService) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	trace, err := qs.spanReader.GetTrace(ctx, traceID)
	if err == spanstore.ErrTraceNotFound {
		if qs.options.ArchiveSpanReader == nil {
			return nil, err
		}
		trace, err = qs.options.ArchiveSpanReader.GetTrace(ctx, traceID)
	}
	return trace, err
}

// GetServices is the queryService implementation of spanstore.Reader.GetServices
func (qs QueryService) GetServices(ctx context.Context) ([]string, error) {
	return qs.spanReader.GetServices(ctx)
}

// GetOperations is the queryService implementation of spanstore.Reader.GetOperations
func (qs QueryService) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	return qs.spanReader.GetOperations(ctx, query)
}

// FindTraces is the queryService implementation of spanstore.Reader.FindTraces
func (qs QueryService) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return qs.spanReader.FindTraces(ctx, query)
}

// ArchiveTrace is the queryService utility to archive traces.
func (qs QueryService) ArchiveTrace(ctx context.Context, traceID model.TraceID) error {
	if qs.options.ArchiveSpanWriter == nil {
		return errNoArchiveSpanStorage
	}
	trace, err := qs.GetTrace(ctx, traceID)
	if err != nil {
		return err
	}

	var writeErrors []error
	for _, span := range trace.Spans {
		err := qs.options.ArchiveSpanWriter.WriteSpan(ctx, span)
		if err != nil {
			writeErrors = append(writeErrors, err)
		}
	}
	return multierror.Wrap(writeErrors)
}

// Adjust applies adjusters to the trace.
func (qs QueryService) Adjust(trace *model.Trace) (*model.Trace, error) {
	return qs.options.Adjuster.Adjust(trace)
}

// GetDependencies implements dependencystore.Reader.GetDependencies
func (qs QueryService) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return qs.dependencyReader.GetDependencies(ctx, endTs, lookback)
}

// InitArchiveStorage tries to initialize archive storage reader/writer if storage factory supports them.
func (opts *QueryServiceOptions) InitArchiveStorage(storageFactory storage.Factory, logger *zap.Logger) bool {
	archiveFactory, ok := storageFactory.(storage.ArchiveFactory)
	if !ok {
		logger.Info("Archive storage not supported by the factory")
		return false
	}
	reader, err := archiveFactory.CreateArchiveSpanReader()
	if err == storage.ErrArchiveStorageNotConfigured || err == storage.ErrArchiveStorageNotSupported {
		logger.Info("Archive storage not created", zap.String("reason", err.Error()))
		return false
	}
	if err != nil {
		logger.Error("Cannot init archive storage reader", zap.Error(err))
		return false
	}
	writer, err := archiveFactory.CreateArchiveSpanWriter()
	if err == storage.ErrArchiveStorageNotConfigured || err == storage.ErrArchiveStorageNotSupported {
		logger.Info("Archive storage not created", zap.String("reason", err.Error()))
		return false
	}
	if err != nil {
		logger.Error("Cannot init archive storage writer", zap.Error(err))
		return false
	}
	opts.ArchiveSpanReader = reader
	opts.ArchiveSpanWriter = writer
	return true
}
