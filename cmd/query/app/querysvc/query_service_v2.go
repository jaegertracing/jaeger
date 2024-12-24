// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc/adjuster"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/iter"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
)

var errNoArchiveSpanStorage = errors.New("archive span storage was not configured")

const (
	defaultMaxClockSkewAdjust = time.Second
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
func NewQueryServiceV2(
	traceReader tracestore.Reader,
	dependencyReader depstore.Reader,
	options QueryServiceOptionsV2,
) *QueryServiceV2 {
	qsvc := &QueryServiceV2{
		traceReader:      traceReader,
		dependencyReader: dependencyReader,
		options:          options,
	}

	if qsvc.options.Adjuster == nil {
		qsvc.options.Adjuster = adjuster.Sequence(adjuster.StandardAdjusters(defaultMaxClockSkewAdjust)...)
	}
	return qsvc
}

// GetTrace is the queryService implementation of tracestore.Reader.GetTrace
func (qs QueryServiceV2) GetTraces(ctx context.Context, traceIDs ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	traceIter := qs.traceReader.GetTraces(ctx, traceIDs...)
	_, err := iter.FlattenWithErrors(traceIter)
	if errors.Is(err, spanstore.ErrTraceNotFound) {
		if qs.options.ArchiveTraceReader == nil {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield(nil, err)
			}
		}
		traceIter = qs.options.ArchiveTraceReader.GetTraces(ctx, traceIDs...)
	}
	return traceIter
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
func (qs QueryServiceV2) FindTraces(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return qs.traceReader.FindTraces(ctx, query)
}

// ArchiveTrace is the queryService utility to archive traces.
func (qs QueryServiceV2) ArchiveTrace(ctx context.Context, traceID pcommon.TraceID) error {
	if qs.options.ArchiveTraceWriter == nil {
		return errNoArchiveSpanStorage
	}
	var err error
	traces, err := iter.FlattenWithErrors(qs.GetTraces(ctx, tracestore.GetTraceParams{TraceID: traceID}))
	if err != nil {
		return err
	}
	for _, trace := range traces {
		err = errors.Join(err, qs.options.ArchiveTraceWriter.WriteTraces(ctx, trace))
	}
	return err
}

// Adjust applies adjusters to the trace.
func (qs QueryServiceV2) Adjust(trace ptrace.Traces) {
	qs.options.Adjuster.Adjust(trace)
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

// hasArchiveStorage returns true if archive storage reader/writer are initialized.
func (opts *QueryServiceOptionsV2) hasArchiveStorage() bool {
	return opts.ArchiveTraceReader != nil && opts.ArchiveTraceWriter != nil
}
