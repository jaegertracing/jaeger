// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc/v2/adjuster"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var errNoArchiveSpanStorage = errors.New("archive span storage was not configured")

// QueryServiceOptions holds the configuration options for the query service.
type QueryServiceOptions struct {
	// ArchiveTraceReader is used to read archived traces from the storage.
	ArchiveTraceReader tracestore.Reader
	// ArchiveTraceWriter is used to write traces to the archive storage.
	ArchiveTraceWriter tracestore.Writer
	// MaxClockSkewAdjust is the maximum duration by which to adjust a span.
	MaxClockSkewAdjust time.Duration
	// MaxTraceSize is the max no. of spans allowed per trace.
	// If a trace has more spans than this, it will be truncated and a warning will be present.
	MaxTraceSize int
}

// StorageCapabilities is a feature flag for query service
type StorageCapabilities struct {
	ArchiveStorage bool `json:"archiveStorage"`
	// TODO: Maybe add metrics Storage here
	// SupportRegex     bool
	// SupportTagFilter bool
}

// QueryService provides methods to query data from the storage.
type QueryService struct {
	traceReader      tracestore.Reader
	dependencyReader depstore.Reader
	adjuster         adjuster.Adjuster
	options          QueryServiceOptions
}

// GetTraceParams defines the parameters for retrieving traces using the GetTraces function.
type GetTraceParams struct {
	// TraceIDs is a slice of trace identifiers to fetch.
	TraceIDs []tracestore.GetTraceParams
	// RawTraces indicates whether to retrieve raw traces.
	// If set to false, the traces will be adjusted using QueryServiceOptions.Adjuster.
	RawTraces bool
}

// TraceQueryParams represents the parameters for querying a batch of traces.
type TraceQueryParams struct {
	tracestore.TraceQueryParams
	// RawTraces indicates whether to retrieve raw traces.
	// If set to false, the traces will be adjusted using QueryServiceOptions.Adjuster.
	RawTraces bool
}

func NewQueryService(
	traceReader tracestore.Reader,
	dependencyReader depstore.Reader,
	options QueryServiceOptions,
) *QueryService {
	qsvc := &QueryService{
		traceReader:      traceReader,
		dependencyReader: dependencyReader,
		adjuster: adjuster.Sequence(
			adjuster.StandardAdjusters(options.MaxClockSkewAdjust)...,
		),
		options: options,
	}

	return qsvc
}

// GetTraces retrieves traces with given trace IDs from the primary reader,
// and if any of them are not found it then queries the archive reader.
// The iterator is single-use: once consumed, it cannot be used again.
func (qs QueryService) GetTraces(
	ctx context.Context,
	params GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	getTracesIter := qs.traceReader.GetTraces(ctx, params.TraceIDs...)
	return func(yield func([]ptrace.Traces, error) bool) {
		foundTraceIDs, proceed := qs.receiveTraces(getTracesIter, yield, params.RawTraces)
		if proceed && qs.options.ArchiveTraceReader != nil {
			var missingTraceIDs []tracestore.GetTraceParams
			for _, id := range params.TraceIDs {
				if _, found := foundTraceIDs[id.TraceID]; !found {
					missingTraceIDs = append(missingTraceIDs, id)
				}
			}
			if len(missingTraceIDs) > 0 {
				getArchiveTracesIter := qs.options.ArchiveTraceReader.GetTraces(ctx, missingTraceIDs...)
				qs.receiveTraces(getArchiveTracesIter, yield, params.RawTraces)
			}
		}
	}
}

func (qs QueryService) GetServices(ctx context.Context) ([]string, error) {
	return qs.traceReader.GetServices(ctx)
}

func (qs QueryService) GetOperations(
	ctx context.Context,
	query tracestore.OperationQueryParams,
) ([]tracestore.Operation, error) {
	return qs.traceReader.GetOperations(ctx, query)
}

func (qs QueryService) FindTraces(
	ctx context.Context,
	query TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		tracesIter := qs.traceReader.FindTraces(ctx, query.TraceQueryParams)
		qs.receiveTraces(tracesIter, yield, query.RawTraces)
	}
}

// ArchiveTrace archives a trace specified by the given query parameters.
// If the ArchiveTraceWriter is not configured, it returns
// an error indicating that there is no archive span storage available.
func (qs QueryService) ArchiveTrace(ctx context.Context, query tracestore.GetTraceParams) error {
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
				archiveErr = errors.Join(archiveErr, err)
			}
		}
		return true
	})
	return archiveErr
}

func (qs QueryService) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return qs.dependencyReader.GetDependencies(ctx, depstore.QueryParameters{
		StartTime: endTs.Add(-lookback),
		EndTime:   endTs,
	})
}

func (qs QueryService) GetCapabilities() StorageCapabilities {
	return StorageCapabilities{
		ArchiveStorage: qs.options.hasArchiveStorage(),
	}
}

func (opts *QueryServiceOptions) hasArchiveStorage() bool {
	return opts.ArchiveTraceReader != nil && opts.ArchiveTraceWriter != nil
}

func (qs QueryService) receiveTraces(
	seq iter.Seq2[[]ptrace.Traces, error],
	yield func([]ptrace.Traces, error) bool,
	rawTraces bool,
) (map[pcommon.TraceID]struct{}, bool) {
	foundTraceIDs := make(map[pcommon.TraceID]struct{})
	proceed := true

	processTraces := func(traces []ptrace.Traces, err error) bool {
		if err != nil {
			proceed = yield(nil, err)
			return proceed
		}
		for _, trace := range traces {
			if !rawTraces {
				qs.adjuster.Adjust(trace)
			}
			jptrace.SpanIter(trace)(func(_ jptrace.SpanIterPos, span ptrace.Span) bool {
				foundTraceIDs[span.TraceID()] = struct{}{}
				return true
			})
		}
		proceed = yield(traces, nil)
		return proceed
	}

	if rawTraces {
		seq(processTraces)
	} else {
		if qs.options.MaxTraceSize > 0 {
			qs.aggregateTracesWithLimit(seq)(func(trace ptrace.Traces, err error) bool { // apply max trace size limit
				return processTraces([]ptrace.Traces{trace}, err)
			})
		} else {
			jptrace.AggregateTraces(seq)(func(trace ptrace.Traces, err error) bool {
				return processTraces([]ptrace.Traces{trace}, err)
			})
		}
	}

	return foundTraceIDs, proceed
}

// It stops aggregating once the limit is reached for a trace and adds a warning.
func (qs QueryService) aggregateTracesWithLimit(
	tracesSeq iter.Seq2[[]ptrace.Traces, error],
) iter.Seq2[ptrace.Traces, error] {
	return func(yield func(trace ptrace.Traces, err error) bool) {
		currentTrace := ptrace.NewTraces()
		currentTraceID := pcommon.NewTraceIDEmpty()
		spanCount := 0
		truncated := false
		skipCurrentTrace := false

		tracesSeq(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				yield(ptrace.NewTraces(), err)
				return false
			}
			for _, trace := range traces {
				resources := trace.ResourceSpans()
				if resources.Len() == 0 {
					continue
				}

				scopes := resources.At(0).ScopeSpans()
				if scopes.Len() == 0 {
					continue
				}

				spans := scopes.At(0).Spans()
				if spans.Len() == 0 {
					continue
				}

				traceID := spans.At(0).TraceID()

				if currentTraceID != traceID {
					if currentTrace.ResourceSpans().Len() > 0 {
						if truncated {
							qs.addTruncationWarning(currentTrace)
						}
						if !yield(currentTrace, nil) {
							return false
						}
					}
					currentTrace = ptrace.NewTraces()
					currentTraceID = traceID
					spanCount = 0
					truncated = false
					skipCurrentTrace = false
				}

				// If already truncated, skip remaining
				if skipCurrentTrace {
					continue
				}

				// Count spans in incoming trace
				incomingSpanCount := 0
				for i := 0; i < resources.Len(); i++ {
					res := resources.At(i)
					for j := 0; j < res.ScopeSpans().Len(); j++ {
						incomingSpanCount += res.ScopeSpans().At(j).Spans().Len()
					}
				}

				// Check if adding these spans would exceed the limit
				if spanCount+incomingSpanCount > qs.options.MaxTraceSize {
					remaining := qs.options.MaxTraceSize - spanCount
					if remaining > 0 {
						qs.copySpansUpToLimit(trace, currentTrace, remaining)
						spanCount = qs.options.MaxTraceSize
					}
					truncated = true
					skipCurrentTrace = true
					continue
				}

				// add all spans from this batch
				mergeTraces(trace, currentTrace)
				spanCount += incomingSpanCount
			}
			return true
		})

		// for last trace if it exists
		if currentTrace.ResourceSpans().Len() > 0 {
			if truncated {
				qs.addTruncationWarning(currentTrace)
			}
			yield(currentTrace, nil)
		}
	}
}

// copies up to 'limit' spans from src to dest.
func (QueryService) copySpansUpToLimit(src, dest ptrace.Traces, limit int) {
	copied := 0
	resources := src.ResourceSpans()

	for i := 0; i < resources.Len() && copied < limit; i++ {
		srcResource := resources.At(i)
		destResource := dest.ResourceSpans().AppendEmpty()
		srcResource.Resource().CopyTo(destResource.Resource())
		destResource.SetSchemaUrl(srcResource.SchemaUrl())

		scopes := srcResource.ScopeSpans()
		for j := 0; j < scopes.Len() && copied < limit; j++ {
			srcScope := scopes.At(j)
			destScope := destResource.ScopeSpans().AppendEmpty()
			srcScope.Scope().CopyTo(destScope.Scope())
			destScope.SetSchemaUrl(srcScope.SchemaUrl())

			spans := srcScope.Spans()
			for k := 0; k < spans.Len() && copied < limit; k++ {
				spans.At(k).CopyTo(destScope.Spans().AppendEmpty())
				copied++
			}
		}
	}
}

// add a warning to the first span of the trace
func (qs QueryService) addTruncationWarning(trace ptrace.Traces) {
	resources := trace.ResourceSpans()
	if resources.Len() == 0 {
		return
	}

	scopes := resources.At(0).ScopeSpans()
	if scopes.Len() == 0 {
		return
	}

	spans := scopes.At(0).Spans()
	if spans.Len() == 0 {
		return
	}

	firstSpan := spans.At(0)
	jptrace.AddWarnings(firstSpan,
		fmt.Sprintf("trace has more than %d spans, showing first %d spans only",
			qs.options.MaxTraceSize, qs.options.MaxTraceSize))
}

// merge src trace into dest trace.
func mergeTraces(src, dest ptrace.Traces) {
	resources := src.ResourceSpans()
	for i := 0; i < resources.Len(); i++ {
		resource := resources.At(i)
		resource.CopyTo(dest.ResourceSpans().AppendEmpty())
	}
}
