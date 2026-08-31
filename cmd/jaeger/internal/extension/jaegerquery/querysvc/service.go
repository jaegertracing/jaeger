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
	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/adjuster"
	"github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var errNoArchiveSpanStorage = errors.New("archive span storage was not configured")

// ErrServiceNameRequired is returned for a search that omits the service name against a
// backend whose reader does not accept one (RFC 0013 §3.3). It names the backend's
// limitation rather than the missing field, because the same query is valid elsewhere.
// The API layers map it to InvalidArgument / HTTP 400.
var ErrServiceNameRequired = errors.New(
	"this storage backend requires a service name to search; searching all services is not supported",
)

// QueryServiceOptions holds the configuration options for the query service.
type QueryServiceOptions struct {
	// ArchiveTraceReader is used to read archived traces from the storage.
	ArchiveTraceReader tracestore.Reader
	// ArchiveTraceWriter is used to write traces to the archive storage.
	ArchiveTraceWriter tracestore.Writer
	// MaxClockSkewAdjust is the maximum duration by which to adjust a span.
	MaxClockSkewAdjust time.Duration
	// MaxTraceSize is the maximum number of spans allowed per trace. A value of 0 (default) means unlimited.
	// If a trace has more spans than this limit, it will be truncated and a warning will be added.
	MaxTraceSize int
	// Interceptors are the query-interceptor extensions this deployment configured, in the order
	// it named them. The query service invokes their OnQuery around every trace search and their
	// OnResult around every batch of loaded traces. Most deployments configure none.
	Interceptors []queryinterceptor.Interceptor
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
	getTracesIter := qs.interceptResults(ctx, qs.traceReader.GetTraces(ctx, params.TraceIDs...))
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
				getArchiveTracesIter := qs.interceptResults(
					ctx, qs.options.ArchiveTraceReader.GetTraces(ctx, missingTraceIDs...),
				)
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
		ctx, query, err := qs.prepareSearchQuery(ctx, query)
		if err != nil {
			yield(nil, err)
			return
		}
		tracesIter := qs.interceptResults(ctx, qs.traceReader.FindTraces(ctx, query.TraceQueryParams))
		qs.receiveTraces(tracesIter, yield, query.RawTraces)
	}
}

// SearchWithoutServiceName reports whether the trace reader accepts a search that omits the
// service name and reads it as "any service" (RFC 0013 §3.3). The reader is asked every time
// rather than once, because a remote backend answers for itself and may not have been
// reachable when jaeger-query started; a reader for which that costs a round trip caches its
// own answer.
//
// A reader that cannot say returns an error, which callers read as the least capable
// backend.
func (qs QueryService) SearchWithoutServiceName(ctx context.Context) (bool, error) {
	caps, err := qs.traceReader.SearchCapabilities(ctx)
	if err != nil {
		return false, err
	}
	return caps.WithoutServiceName, nil
}

// prepareSearchQuery settles a search before it is dispatched: it refuses a request this
// deployment does not accept, gives the configured query interceptors their say, and returns the
// query to dispatch in the shape the backend understands, along with the context to dispatch it
// with. One place decides, so that every caller gets the same answer instead of each backend's
// own (ADR-013).
//
// The interceptors run after the caller's request is validated and before the backend's
// capabilities are consulted. So an interceptor is never shown a request jaeger-query was going
// to refuse anyway, and a predicate an interceptor adds is held to the same capability check as
// one the caller sent.
func (qs QueryService) prepareSearchQuery(
	ctx context.Context,
	query TraceQueryParams,
) (context.Context, TraceQueryParams, error) {
	if query.Filter != nil {
		// None of these refusals depends on the backend, so they come before the capability call
		// rather than after it.
		if !StructuredFiltersGate.IsEnabled() {
			return ctx, query, fmt.Errorf("%w: enable the %q feature gate to use it",
				ErrFilterDisabled, StructuredFiltersGate.ID())
		}
		if err := query.EnsureFilterStandsAlone(); err != nil {
			return ctx, query, err
		}
		// Decoding a filter validates nothing, so it is finalized — validated and normalized —
		// here, on behalf of every API layer above (RFC 0005 §7).
		finalized, err := expression.Finalize(query.Filter)
		if err != nil {
			return ctx, query, fmt.Errorf("%w: %w", tracestore.ErrFilterInvalid, err)
		}
		query.Filter = finalized
	}
	if len(qs.options.Interceptors) > 0 {
		var err error
		ctx, query, err = qs.onQuery(ctx, query)
		if err != nil {
			return ctx, query, err
		}
	}
	if query.Filter == nil {
		return ctx, query, qs.checkServiceName(ctx, query)
	}
	caps, err := qs.traceReader.SearchCapabilities(ctx)
	if err != nil {
		// A reader that cannot report its capabilities reads as the least capable one, which
		// serves only the legacy predicate fields.
		caps = tracestore.SearchCapabilities{}
	}
	// The filter is settled before the service name is checked, because a filter can name the
	// service itself and rewriting it is what moves that into ServiceName.
	prepared, err := query.ForCapabilities(caps)
	if err != nil {
		return ctx, query, err
	}
	query.TraceQueryParams = prepared
	if query.ServiceName == "" && !caps.WithoutServiceName {
		return ctx, query, ErrServiceNameRequired
	}
	return ctx, query, nil
}

func (qs QueryService) checkServiceName(ctx context.Context, query TraceQueryParams) error {
	if query.ServiceName != "" {
		return nil
	}
	if withoutServiceName, err := qs.SearchWithoutServiceName(ctx); err != nil || !withoutServiceName {
		return ErrServiceNameRequired
	}
	return nil
}

// FindTraceSummaries searches for traces matching the query and returns an iterator
// of lightweight summary information. It calls the trace reader's FindTraceSummaries;
// readers that cannot compute summaries natively yield errors.ErrUnsupported (wrapped
// with %w) as the first error, in which case FindTraceSummaries transparently falls
// back to FindTraces and computes summaries from the full trace data.
//
// The iterator is single-use: once consumed, it cannot be used again.
func (qs QueryService) FindTraceSummaries(
	ctx context.Context,
	query TraceQueryParams,
) iter.Seq2[[]tracestore.TraceSummary, error] {
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		ctx, query, err := qs.prepareSearchQuery(ctx, query)
		if err != nil {
			yield(nil, err)
			return
		}
		for batch, err := range qs.traceReader.FindTraceSummaries(ctx, query.TraceQueryParams) {
			if err != nil {
				if errors.Is(err, errors.ErrUnsupported) {
					// Fall back to FindTraces + aggregation. The fallback loads whole traces, so
					// the interceptors get the same say over them as on a FindTraces search; the
					// summaries computed from them carry no spans and have no hook of their own.
					traces := qs.interceptResults(ctx, qs.traceReader.FindTraces(ctx, query.TraceQueryParams))
					for b, e := range computeSummaries(traces, qs.adjuster) {
						if !yield(b, e) {
							return
						}
					}
					return
				}
				yield(nil, err)
				return
			}
			if !yield(batch, nil) {
				return
			}
		}
	}
}

// ArchiveTrace archives a trace specified by the given query parameters.
// If the ArchiveTraceWriter is not configured, it returns
// an error indicating that there is no archive span storage available.
func (qs QueryService) ArchiveTrace(ctx context.Context, query tracestore.GetTraceParams) error {
	if qs.options.ArchiveTraceWriter == nil {
		return errNoArchiveSpanStorage
	}
	getTracesIter := qs.interceptResults(ctx, qs.traceReader.GetTraces(ctx, query))
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
		jptrace.AggregateTracesWithLimit(seq, qs.options.MaxTraceSize)(func(trace ptrace.Traces, err error) bool {
			return processTraces([]ptrace.Traces{trace}, err)
		})
	}

	return foundTraceIDs, proceed
}
