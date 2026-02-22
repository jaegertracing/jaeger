// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"errors"
	"iter"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/adjuster"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
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
	// MaxTraceSize is the maximum number of spans to load per trace.
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
//
// Returned iterator behavior:
//   - When RawTraces is false (default), each returned ptrace.Traces object contains
//     a complete, aggregated trace. If the underlying storage returns a trace split
//     across multiple consecutive ptrace.Traces chunks (per tracestore.Reader contract),
//     they will be aggregated into a single ptrace.Traces object.
//   - When RawTraces is true, ptrace.Traces chunks are returned as-is from storage
//     without aggregation or adjustment. A single trace may be split across multiple
//     consecutive ptrace.Traces objects.
//   - Archive reader traces (if any) are processed the same way and yielded after
//     all primary reader traces.
func (qs QueryService) GetTraces(
	ctx context.Context,
	params GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	getTracesIter := qs.limitTraceSize(qs.traceReader.GetTraces(ctx, params.TraceIDs...))
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
	services, err := qs.traceReader.GetServices(ctx)
	if services == nil {
		services = []string{}
	}
	return services, err
}

func (qs QueryService) GetOperations(
	ctx context.Context,
	query tracestore.OperationQueryParams,
) ([]tracestore.Operation, error) {
	return qs.traceReader.GetOperations(ctx, query)
}

// FindTraces searches for traces matching the query parameters.
// The iterator is single-use: once consumed, it cannot be used again.
//
// Returned iterator behavior:
//   - When RawTraces is false (default), each returned ptrace.Traces object contains
//     a complete, aggregated trace. If the underlying storage returns a trace split
//     across multiple consecutive ptrace.Traces chunks (per tracestore.Reader contract),
//     they will be aggregated into a single ptrace.Traces object.
//   - When RawTraces is true, ptrace.Traces chunks are returned as-is from storage
//     without aggregation or adjustment. A single trace may be split across multiple
//     consecutive ptrace.Traces objects.
func (qs QueryService) FindTraces(
	ctx context.Context,
	query TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		tracesIter := qs.limitTraceSize(qs.traceReader.FindTraces(ctx, query.TraceQueryParams))
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
	var (
		found      bool
		archiveErr error
	)
	getTracesIter(func(traces []ptrace.Traces, err error) bool {
		if err != nil {
			archiveErr = err
			return false
		}
		for _, trace := range traces {
			found = true
			err = qs.options.ArchiveTraceWriter.WriteTraces(ctx, trace)
			if err != nil {
				archiveErr = errors.Join(archiveErr, err)
			}
		}
		return true
	})
	if archiveErr == nil && !found {
		return spanstore.ErrTraceNotFound
	}
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
		jptrace.AggregateTraces(seq)(func(trace ptrace.Traces, err error) bool {
			return processTraces([]ptrace.Traces{trace}, err)
		})
	}

	return foundTraceIDs, proceed
}

func (qs QueryService) limitTraceSize(seq iter.Seq2[[]ptrace.Traces, error]) iter.Seq2[[]ptrace.Traces, error] {
	if qs.options.MaxTraceSize <= 0 {
		return seq
	}
	return func(yield func([]ptrace.Traces, error) bool) {
		spanCounts := make(map[pcommon.TraceID]int)
		exceeded := make(map[pcommon.TraceID]bool)

		seq(func(traces []ptrace.Traces, err error) bool {
			if err != nil {
				return yield(nil, err)
			}
			var validTraces []ptrace.Traces
			for _, trace := range traces {
				if trace.SpanCount() == 0 {
					continue
				}

				// Count spans per TraceID within this ptrace.Traces and
				// remember a representative span for each TraceID for warnings.
				traceSpanCounts := make(map[pcommon.TraceID]int)
				firstSpanByTraceID := make(map[pcommon.TraceID]ptrace.Span)

				resources := trace.ResourceSpans()
				for i := 0; i < resources.Len(); i++ {
					scopeSpans := resources.At(i).ScopeSpans()
					for j := 0; j < scopeSpans.Len(); j++ {
						spans := scopeSpans.At(j).Spans()
						for k := 0; k < spans.Len(); k++ {
							span := spans.At(k)
							traceID := span.TraceID()
							// Skip spans without a TraceID.
							if traceID.IsEmpty() {
								continue
							}
							traceSpanCounts[traceID]++
							if _, ok := firstSpanByTraceID[traceID]; !ok {
								firstSpanByTraceID[traceID] = span
							}
						}
					}
				}

				if len(traceSpanCounts) == 0 {
					continue
				}

				// Determine if at least one TraceID in this chunk is still under the limit.
				hasNonExceededTrace := false
				for traceID, countInChunk := range traceSpanCounts {
					if exceeded[traceID] {
						// This trace has already exceeded the limit; ignore further spans.
						continue
					}

					currentCount := spanCounts[traceID]
					if currentCount+countInChunk > qs.options.MaxTraceSize {
						exceeded[traceID] = true
						// Attach a warning to the first span for this traceID in the chunk
						spanToKeep, ok := firstSpanByTraceID[traceID]
						if ok {
							jptrace.AddWarnings(spanToKeep, "trace size exceeded maximum allowed size")
							warnTrace := ptrace.NewTraces()
							rs := warnTrace.ResourceSpans().AppendEmpty()
							ss := rs.ScopeSpans().AppendEmpty()
							spanToKeep.CopyTo(ss.Spans().AppendEmpty())
							validTraces = append(validTraces, warnTrace)
						}
						continue
					}

					spanCounts[traceID] = currentCount + countInChunk
					hasNonExceededTrace = true
				}

				if hasNonExceededTrace {
					validTraces = append(validTraces, trace)
				}
			}

			return yield(validTraces, nil)
		})
	}
}
