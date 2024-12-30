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
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/iter"
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
type StorageCapabilities struct {
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

// GetTraceParams defines the parameters for retrieving traces using the GetTraces function.
type GetTraceParams struct {
	// TraceIDs is a slice of trace identifiers to fetch.
	TraceIDs []tracestore.GetTraceParams
	// RawTraces indicates whether to retrieve raw traces.
	// If set to false, the traces will be adjusted using QueryServiceOptionsV2.Adjuster.
	RawTraces bool
}

// TraceQueryParams represents the parameters for querying a batch of traces.
type TraceQueryParams struct {
	tracestore.TraceQueryParams
	// RawTraces indicates whether to retrieve raw traces.
	// If set to false, the traces will be adjusted using QueryServiceOptionsV2.Adjuster.
	RawTraces bool
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
		qsvc.options.Adjuster = adjuster.Sequence(
			adjuster.StandardAdjusters(defaultMaxClockSkewAdjust)...)
	}
	return qsvc
}

// GetTraces retrieves traces with given trace IDs from the primary reader,
// and if any of them are not found it then queries the archive reader.
// The iterator is single-use: once consumed, it cannot be used again.
func (qs QueryServiceV2) GetTraces(
	ctx context.Context,
	params GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	getTracesIter := qs.traceReader.GetTraces(ctx, params.TraceIDs...)
	aggregatedTraces := jptrace.AggregateTraces(getTracesIter)
	return func(yield func([]ptrace.Traces, error) bool) {
		foundTraceIDs := make(map[pcommon.TraceID]struct{})
		proceed := true
		aggregatedTraces(func(trace ptrace.Traces, err error) bool {
			if err != nil {
				proceed = yield(nil, err)
				return proceed
			}
			if !params.RawTraces {
				qs.options.Adjuster.Adjust(trace)
			}
			jptrace.SpanIter(trace)(func(_ jptrace.SpanIterPos, span ptrace.Span) bool {
				foundTraceIDs[span.TraceID()] = struct{}{}
				return true
			})
			proceed = yield([]ptrace.Traces{trace}, nil)
			return proceed
		})
		if proceed && qs.options.ArchiveTraceReader != nil {
			var missingTraceIDs []tracestore.GetTraceParams
			for _, id := range params.TraceIDs {
				if _, found := foundTraceIDs[id.TraceID]; !found {
					missingTraceIDs = append(missingTraceIDs, id)
				}
			}
			if len(missingTraceIDs) > 0 {
				getArchiveTracesIter := qs.options.ArchiveTraceReader.GetTraces(ctx, missingTraceIDs...)
				aggregatedArchiveTraces := jptrace.AggregateTraces(getArchiveTracesIter)
				aggregatedArchiveTraces(func(trace ptrace.Traces, err error) bool {
					if err != nil {
						return yield(nil, err)
					}
					if !params.RawTraces {
						qs.options.Adjuster.Adjust(trace)
					}
					return yield([]ptrace.Traces{trace}, nil)
				})
			}
		}
	}
}

// GetServices is the queryService implementation of tracestore.Reader.GetServices
func (qs QueryServiceV2) GetServices(ctx context.Context) ([]string, error) {
	return qs.traceReader.GetServices(ctx)
}

// GetOperations is the queryService implementation of tracestore.Reader.GetOperations
func (qs QueryServiceV2) GetOperations(
	ctx context.Context,
	query tracestore.OperationQueryParams,
) ([]tracestore.Operation, error) {
	return qs.traceReader.GetOperations(ctx, query)
}

// FindTraces is the queryService implementation of tracestore.Reader.FindTraces
func (qs QueryServiceV2) FindTraces(
	ctx context.Context,
	query TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		tracesIter := qs.traceReader.FindTraces(ctx, query.TraceQueryParams)
		tracesIter(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				return yield(nil, err)
			}
			if !query.RawTraces {
				for _, trace := range traces {
					qs.options.Adjuster.Adjust(trace)
				}
			}
			return yield(traces, nil)
		})
	}
}

// ArchiveTrace is the queryService utility to archive traces.
func (qs QueryServiceV2) ArchiveTrace(ctx context.Context, query tracestore.GetTraceParams) error {
	if qs.options.ArchiveTraceWriter == nil {
		return errNoArchiveSpanStorage
	}
	getTracesIter := qs.GetTraces(
		ctx, GetTraceParams{TraceIDs: []tracestore.GetTraceParams{query}},
	)
	var archiveErr error
	getTracesIter(func(traces []ptrace.Traces, err error) bool {
		if err != nil {
			archiveErr = err
			return false
		}
		for _, trace := range traces {
			err = qs.options.ArchiveTraceWriter.WriteTraces(ctx, trace)
			if err != nil {
				archiveErr = err
				return false
			}
		}
		return true
	})
	return archiveErr
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
