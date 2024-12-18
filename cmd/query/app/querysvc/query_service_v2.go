// // Copyright (c) 2019 The Jaeger Authors.
// // SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
)

// TODO: Remove query_service.go and rename query_service_v2.go
// to query_service.go once all components have been migrated to
// operate on the OTEL data model.
var errNoArchiveSpanStorageV2 = errors.New("archive span storage was not configured")

const (
	defaultMaxClockSkewAdjustV2 = time.Second
)

// QueryServiceOptions has optional members of QueryService
type QueryServiceOptionsV2 struct {
	ArchiveTraceReader tracestore.Reader
	ArchiveTraceWriter tracestore.Writer
	Adjuster           adjuster.Adjuster
}

// StorageCapabilities is a feature flag for query service
type StorageCapabilitiesV2 struct {
	ArchiveStorage bool `json:"archiveStorage"`
	// TODO: Maybe add metrics Storage here
	// SupportRegex     bool
	// SupportTagFilter bool
}

// QueryService contains span utils required by the query-service.
type QueryServiceV2 struct {
	traceReader      tracestore.Reader
	dependencyReader depstore.Reader
	options          QueryServiceOptionsV2
}

// NewQueryService returns a new QueryService.
func NewQueryServiceV2(traceReader tracestore.Reader, dependencyReader depstore.Reader, options QueryServiceOptions) *QueryService {
	qsvc := &QueryService{
		traceReader:      traceReader,
		dependencyReader: dependencyReader,
		options:          options,
	}

	if qsvc.options.Adjuster == nil {
		qsvc.options.Adjuster = adjuster.Sequence(StandardAdjusters(defaultMaxClockSkewAdjustV2)...)
	}
	return qsvc
}

// GetTrace is the queryService implementation of tracestore.Reader.GetTrace
func (qs QueryServiceV2) GetTrace(ctx context.Context, traceID pcommon.TraceID) (ptrace.Traces, error) {
	trace, err := qs.traceReader.GetTrace(ctx, traceID)
	if errors.Is(err, spanstore.ErrTraceNotFound) {
		if qs.options.ArchiveTraceReader == nil {
			return ptrace.NewTraces(), err
		}
		trace, err = qs.options.ArchiveTraceReader.GetTrace(ctx, traceID)
	}
	return trace, err
}

// GetServices is the queryService implementation of tracestore.Reader.GetServices
func (qs QueryServiceV2) GetServices(ctx context.Context) ([]string, error) {
	return qs.traceReader.GetServices(ctx)
}

// GetOperations is the queryService implementation of tracestore.Reader.GetOperations
func (qs QueryServiceV2) GetOperations(
	ctx context.Context,
	query tracestore.OperationQueryParameters,
) ([]tracestore.Operation, error) {
	return qs.traceReader.GetOperations(ctx, query)
}

// FindTraces is the queryService implementation of tracestore.Reader.FindTraces
func (qs QueryServiceV2) FindTraces(ctx context.Context, query tracestore.TraceQueryParameters) ([]ptrace.Traces, error) {
	return qs.traceReader.FindTraces(ctx, query)
}

// ArchiveTrace is the queryService utility to archive traces.
func (qs QueryServiceV2) ArchiveTrace(ctx context.Context, traceID pcommon.TraceID) error {
	if qs.options.ArchiveTraceWriter == nil {
		return errNoArchiveSpanStorageV2
	}
	trace, err := qs.GetTrace(ctx, traceID)
	if err != nil {
		return err
	}
	return qs.options.ArchiveTraceWriter.WriteTraces(ctx, trace)
}

// Adjust applies adjusters to the trace.
func (qs QueryServiceV2) Adjust(trace *model.Trace) (*model.Trace, error) {
	return qs.options.Adjuster.Adjust(trace)
}

// GetDependencies implements depstore.Reader.GetDependencies
func (qs QueryServiceV2) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return qs.dependencyReader.GetDependencies(ctx, depstore.QueryParameters{
		StartTime: endTs.Add(-lookback),
		EndTime:   endTs,
	})
}

// GetCapabilities returns the features supported by the query service.
func (qs QueryServiceV2) GetCapabilities() StorageCapabilities {
	return StorageCapabilities{
		ArchiveStorage: qs.options.hasArchiveStorage(),
	}
}

// InitArchiveStorage tries to initialize archive storage reader/writer if storage factory supports them.
func (opts *QueryServiceOptionsV2) InitArchiveStorage(
	archiveReader tracestore.Reader,
	archiveWriter tracestore.Writer,
	logger *zap.Logger) {
	opts.ArchiveTraceReader = archiveReader
	opts.ArchiveTraceWriter = archiveWriter
}

// hasArchiveStorage returns true if archive storage reader/writer are initialized.
func (opts *QueryServiceOptionsV2) hasArchiveStorage() bool {
	return opts.ArchiveTraceReader != nil && opts.ArchiveTraceWriter != nil
}
