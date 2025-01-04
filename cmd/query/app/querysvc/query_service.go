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
	spanReader       spanstore.Reader
	dependencyReader depstore.Reader
	options          QueryServiceOptions
}

// GetTraceParameters defines the parameters for querying a single trace from the query service.
type GetTraceParameters struct {
	spanstore.GetTraceParameters
	RawTraces bool
}

// TraceQueryParameters defines the parameters for querying a batch of traces from the query service.
type TraceQueryParameters struct {
	spanstore.TraceQueryParameters
	RawTraces bool
}

// NewQueryService returns a new QueryService.
func NewQueryService(traceReader tracestore.Reader, dependencyReader depstore.Reader, options QueryServiceOptions) *QueryService {
	spanReader, err := v1adapter.GetV1Reader(traceReader)
	if err != nil {
		// TODO: implement a reverse adapter to convert v2 reader to v1 reader
		panic("cannot convert v2 reader to v1 reader: " + err.Error())
	}
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
func (qs QueryService) GetTrace(ctx context.Context, query GetTraceParameters) (*model.Trace, error) {
	trace, err := qs.spanReader.GetTrace(ctx, query.GetTraceParameters)
	if errors.Is(err, spanstore.ErrTraceNotFound) {
		if qs.options.ArchiveSpanReader == nil {
			return nil, err
		}
		trace, err = qs.options.ArchiveSpanReader.GetTrace(ctx, query.GetTraceParameters)
	}
	if err != nil {
		return nil, err
	}
	if !query.RawTraces {
		qs.adjust(trace)
	}
	return trace, nil
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
func (qs QueryService) FindTraces(ctx context.Context, query *TraceQueryParameters) ([]*model.Trace, error) {
	traces, err := qs.spanReader.FindTraces(ctx, &query.TraceQueryParameters)
	if err != nil {
		return nil, err
	}
	if !query.RawTraces {
		for _, trace := range traces {
			qs.adjust(trace)
		}
	}
	return traces, nil
}

// ArchiveTrace is the queryService utility to archive traces.
func (qs QueryService) ArchiveTrace(ctx context.Context, query spanstore.GetTraceParameters) error {
	if qs.options.ArchiveSpanWriter == nil {
		return errNoArchiveSpanStorage
	}
	trace, err := qs.GetTrace(ctx, GetTraceParameters{GetTraceParameters: query})
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
func (qs QueryService) adjust(trace *model.Trace) {
	qs.options.Adjuster.Adjust(trace)
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
