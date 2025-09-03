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
	// MaxTraceSize is the maximum number of spans per trace (0 = no limit, truncates with warning if exceeded)
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
			// Apply max trace size limit if configured
			if qs.options.MaxTraceSize > 0 {
				trace = qs.limitTraceSize(trace)
			}

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
		jptrace.AggregateTraces(seq)(func(trace ptrace.Traces, err error) bool {
			return processTraces([]ptrace.Traces{trace}, err)
		})
	}

	return foundTraceIDs, proceed
}

// limitTraceSize limits the number of spans in a trace to the configured maximum.
// If the trace exceeds the limit, it will be truncated and a warning will be added.
func (qs QueryService) limitTraceSize(trace ptrace.Traces) ptrace.Traces {
	totalSpans := 0
	for i := 0; i < trace.ResourceSpans().Len(); i++ {
		resource := trace.ResourceSpans().At(i)
		for j := 0; j < resource.ScopeSpans().Len(); j++ {
			scope := resource.ScopeSpans().At(j)
			totalSpans += scope.Spans().Len()
		}
	}

	if totalSpans <= qs.options.MaxTraceSize {
		return trace
	}

	// Create a new trace with limited spans
	limitedTrace := ptrace.NewTraces()
	spansAdded := 0

	for i := 0; i < trace.ResourceSpans().Len() && spansAdded < qs.options.MaxTraceSize; i++ {
		resource := trace.ResourceSpans().At(i)

		// Check if we'll keep any spans from this resource before creating it
		resourceHasSpansToKeep := false
		remainingSpans := qs.options.MaxTraceSize - spansAdded

		// Preview if any spans will be kept from this resource
		for j := 0; j < resource.ScopeSpans().Len() && remainingSpans > 0; j++ {
			scope := resource.ScopeSpans().At(j)
			spansInScope := scope.Spans().Len()
			if spansInScope > 0 {
				resourceHasSpansToKeep = true
				break
			}
		}

		// Only create a new resource if it will contain spans
		if resourceHasSpansToKeep {
			newResource := limitedTrace.ResourceSpans().AppendEmpty()
			resource.Resource().CopyTo(newResource.Resource())

			// Limit spans in this resource
			for j := 0; j < resource.ScopeSpans().Len() && spansAdded < qs.options.MaxTraceSize; j++ {
				scope := resource.ScopeSpans().At(j)
				spans := scope.Spans()
				spansToKeep := qs.options.MaxTraceSize - spansAdded
				if spansToKeep > spans.Len() {
					spansToKeep = spans.Len()
				}

				if spansToKeep > 0 {
					newScope := newResource.ScopeSpans().AppendEmpty()
					scope.Scope().CopyTo(newScope.Scope())
					newScope.SetSchemaUrl(scope.SchemaUrl())

					// Copy only the first spansToKeep spans
					for k := 0; k < spansToKeep; k++ {
						span := spans.At(k)
						newSpan := newScope.Spans().AppendEmpty()
						span.CopyTo(newSpan)
						spansAdded++
					}
				}
			}
		}
	}

	// Add warning attribute to the first span
	if limitedTrace.ResourceSpans().Len() > 0 {
		firstResource := limitedTrace.ResourceSpans().At(0)
		if firstResource.ScopeSpans().Len() > 0 {
			firstScope := firstResource.ScopeSpans().At(0)
			if firstScope.Spans().Len() > 0 {
				firstSpan := firstScope.Spans().At(0)
				firstSpan.Attributes().PutStr("jaeger.warning", fmt.Sprintf("Trace truncated: only first %d spans loaded (total spans: %d)", qs.options.MaxTraceSize, totalSpans))
			}
		}
	}

	return limitedTrace
}
