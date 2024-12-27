// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

var errNoArchiveSpanStorage = errors.New("archive span storage was not configured")

const (
	defaultMaxClockSkewAdjust = time.Second
)

// QueryServiceOptions has optional members of QueryService
type QueryServiceOptions struct {
	ArchiveSpanReader spanstore.Reader
	ArchiveSpanWriter spanstore.Writer
	Adjuster          adjuster.Adjuster
}

// StorageCapabilities is a feature flag for query service
type StorageCapabilities struct {
	ArchiveStorage bool `json:"archiveStorage"`
	// TODO: Maybe add metrics Storage here
	// SupportRegex     bool
	// SupportTagFilter bool
}

// QueryService contains span utils required by the query-service.
type QueryService struct {
	traceReader      tracestore.Reader
	dependencyReader depstore.Reader
	options          QueryServiceOptions
}

// NewQueryService returns a new QueryService.
func NewQueryService(traceReader tracestore.Reader, dependencyReader depstore.Reader, options QueryServiceOptions) *QueryService {
	qsvc := &QueryService{
		traceReader:      traceReader,
		dependencyReader: dependencyReader,
		options:          options,
	}

	if qsvc.options.Adjuster == nil {
		qsvc.options.Adjuster = adjuster.Sequence(StandardAdjusters(defaultMaxClockSkewAdjust)...)
	}
	return qsvc
}

// GetTrace is the queryService implementation of spanstore.Reader.GetTrace
func (qs QueryService) GetTrace(ctx context.Context, query spanstore.GetTraceParameters) (*model.Trace, error) {
	spanReader, err := v1adapter.GetV1Reader(qs.traceReader)
	if err != nil {
		return nil, err
	}
	trace, err := spanReader.GetTrace(ctx, query)
	if errors.Is(err, spanstore.ErrTraceNotFound) {
		if qs.options.ArchiveSpanReader == nil {
			return nil, err
		}
		trace, err = qs.options.ArchiveSpanReader.GetTrace(ctx, query)
	}
	if !query.RawTraces {
		trace, err = qs.Adjust(trace)
	}
	return trace, err
}

// GetServices is the queryService implementation of spanstore.Reader.GetServices
func (qs QueryService) GetServices(ctx context.Context) ([]string, error) {
	spanReader, err := v1adapter.GetV1Reader(qs.traceReader)
	if err != nil {
		return nil, err
	}
	return spanReader.GetServices(ctx)
}

// GetOperations is the queryService implementation of spanstore.Reader.GetOperations
func (qs QueryService) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	spanReader, err := v1adapter.GetV1Reader(qs.traceReader)
	if err != nil {
		return nil, err
	}
	return spanReader.GetOperations(ctx, query)
}

// FindTraces is the queryService implementation of spanstore.Reader.FindTraces
func (qs QueryService) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	spanReader, err := v1adapter.GetV1Reader(qs.traceReader)
	if err != nil {
		return nil, err
	}
	traces, err := spanReader.FindTraces(ctx, query)
	if !query.RawTraces {
		for _, trace := range traces {
			_, err := qs.Adjust(trace)
			if err != nil {
				return nil, err
			}
		}
	}
	return traces, err
}

// ArchiveTrace is the queryService utility to archive traces.
func (qs QueryService) ArchiveTrace(ctx context.Context, query spanstore.GetTraceParameters) error {
	if qs.options.ArchiveSpanWriter == nil {
		return errNoArchiveSpanStorage
	}
	trace, err := qs.GetTrace(ctx, query)
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
	return errors.Join(writeErrors...)
}

// Adjust applies adjusters to the trace.
func (qs QueryService) Adjust(trace *model.Trace) (*model.Trace, error) {
	return qs.options.Adjuster.Adjust(trace)
}

// GetDependencies implements dependencystore.Reader.GetDependencies
func (qs QueryService) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return qs.dependencyReader.GetDependencies(ctx, depstore.QueryParameters{
		StartTime: endTs.Add(-lookback),
		EndTime:   endTs,
	})
}

// GetCapabilities returns the features supported by the query service.
func (qs QueryService) GetCapabilities() StorageCapabilities {
	return StorageCapabilities{
		ArchiveStorage: qs.options.hasArchiveStorage(),
	}
}

// InitArchiveStorage tries to initialize archive storage reader/writer if storage factory supports them.
func (opts *QueryServiceOptions) InitArchiveStorage(storageFactory storage.BaseFactory, logger *zap.Logger) bool {
	archiveFactory, ok := storageFactory.(storage.ArchiveFactory)
	if !ok {
		logger.Info("Archive storage not supported by the factory")
		return false
	}
	reader, err := archiveFactory.CreateArchiveSpanReader()
	if errors.Is(err, storage.ErrArchiveStorageNotConfigured) || errors.Is(err, storage.ErrArchiveStorageNotSupported) {
		logger.Info("Archive storage not created", zap.String("reason", err.Error()))
		return false
	}
	if err != nil {
		logger.Error("Cannot init archive storage reader", zap.Error(err))
		return false
	}
	writer, err := archiveFactory.CreateArchiveSpanWriter()
	if errors.Is(err, storage.ErrArchiveStorageNotConfigured) || errors.Is(err, storage.ErrArchiveStorageNotSupported) {
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

// hasArchiveStorage returns true if archive storage reader/writer are initialized.
func (opts *QueryServiceOptions) hasArchiveStorage() bool {
	return opts.ArchiveSpanReader != nil && opts.ArchiveSpanWriter != nil
}
