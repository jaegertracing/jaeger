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

// DefaultSearchDepth bounds a trace search whose caller left SearchDepth unset. It is applied
// here rather than in each API handler, so a gRPC and an HTTP client get the same bound.
const DefaultSearchDepth = 100

// DefaultPageSize bounds a span search whose caller left the page size unset. A span query has
// no SearchDepth, so the page size is its only bound (RFC 0016 §6), and a Reader never receives
// a query without one. It is applied here for the same reason as DefaultSearchDepth.
const DefaultPageSize = DefaultSearchDepth

// ErrQueryInvalid is returned for a trace search whose envelope is malformed on its own terms:
// a missing or inverted time range, a negative or inverted duration bound, or a search depth
// outside [0, MaxSearchDepth]. None of these depends on the backend. The API layers map it to
// InvalidArgument / HTTP 400.
var ErrQueryInvalid = errors.New("invalid query")

// ErrSpanSearchUnsupported is returned for a span search against a backend whose reader does
// not declare SpanSearch (RFC 0016 §4.5). It names the backend's limitation, because the same
// query is valid elsewhere. The interceptor package has a sentinel of the same name for an
// interceptor with no span-search policy; that one is a deployment fault, not a bad request.
var ErrSpanSearchUnsupported = errors.New("this storage backend does not declare span search support")

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
	// it named them. The query service invokes their OnTraceQuery around every trace search and their
	// OnTraceResult around every batch of loaded traces. Most deployments configure none.
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

// SpanQueryParams represents the parameters for a span search (RFC 0016).
type SpanQueryParams struct {
	tracestore.SpanQueryParams
}

// TraceQueryParams represents the parameters for querying a batch of traces.
type TraceQueryParams struct {
	tracestore.TraceQueryParams
	// RawTraces indicates whether to retrieve raw traces.
	// If set to false, the traces will be adjusted using QueryServiceOptions.Adjuster.
	RawTraces bool
}

// PageChunk carries one streamed chunk of a page without exposing the storage
// API's result container to query-service consumers. NextPageToken is the token a
// client sends back to continue, not the reader's cursor (RFC 0014 §3.1). It is set
// only on the final chunk; an empty token there means no later page, while an empty
// token on an earlier chunk says nothing about pagination.
type PageChunk[T any] struct {
	Results       T
	NextPageToken string
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
	getTracesIter := qs.interceptTraceResults(ctx, qs.traceReader.GetTraces(ctx, params.TraceIDs...))
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
				getArchiveTracesIter := qs.interceptTraceResults(
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

// FindSpans searches for spans matching the query parameters.
// The iterator is single-use: once consumed, it cannot be used again.
func (qs QueryService) FindSpans(
	ctx context.Context,
	query SpanQueryParams,
) iter.Seq2[PageChunk[ptrace.Traces], error] {
	return func(yield func(PageChunk[ptrace.Traces], error) bool) {
		ctx, query, err := qs.prepareAndInterceptSpanSearchQuery(ctx, query)
		if err != nil {
			yield(PageChunk[ptrace.Traces]{}, err)
			return
		}
		minter, err := resumeSpanSearch(&query.SpanQueryParams)
		if err != nil {
			yield(PageChunk[ptrace.Traces]{}, err)
			return
		}
		spans := qs.traceReader.FindSpans(ctx, query.SpanQueryParams)
		for chunk, err := range qs.interceptSpanResults(ctx, spans) {
			result := PageChunk[ptrace.Traces]{Results: chunk.Results}
			if err == nil {
				result.NextPageToken, err = minter.mint(chunk.NextPageToken)
			}
			if !yield(result, err) {
				return
			}
		}
	}
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
		// The FindTraces response has no field for a continuation token (RFC 0014 §4).
		if query.Pagination != nil {
			yield(nil, tracestore.ErrPaginationUnsupportedByFindTraces)
			return
		}
		ctx, query, err := qs.prepareAndInterceptSearchQuery(ctx, query)
		if err != nil {
			yield(nil, err)
			return
		}
		tracesIter := qs.interceptTraceResults(ctx, qs.traceReader.FindTraces(ctx, query.TraceQueryParams))
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

// prepareAndInterceptSearchQuery settles a search before it is dispatched: it refuses a request
// this deployment does not accept, runs the configured query interceptors (OnTraceQuery) over it,
// and returns the query to dispatch in the shape the backend understands, along with the context
// the interceptors returned. One place decides, so that every caller gets the same answer instead
// of each backend's own (ADR-013).
//
// The interceptors run after the caller's request is validated and before the backend's
// capabilities are consulted. So an interceptor is never shown a request jaeger-query was going
// to refuse anyway, and a predicate an interceptor adds is held to the same capability check as
// one the caller sent.
func (qs QueryService) prepareAndInterceptSearchQuery(
	ctx context.Context,
	query TraceQueryParams,
) (context.Context, TraceQueryParams, error) {
	if err := query.normalizeEnvelope(); err != nil {
		return ctx, query, err
	}
	if query.Pagination != nil {
		if !PaginationGate.IsEnabled() {
			return ctx, query, fmt.Errorf("%w: enable the %q feature gate to use it",
				ErrPaginationDisabled, PaginationGate.ID())
		}
		// A page size replaces the search depth rather than falling back to it (RFC 0014 §4).
		if query.SearchDepth != 0 {
			return ctx, query, fmt.Errorf("%w: it cannot be combined with search depth",
				tracestore.ErrPaginationInvalid)
		}
		if query.Pagination.PageSize <= 0 {
			return ctx, query, fmt.Errorf("%w: page size is required whenever pagination is present",
				tracestore.ErrPaginationInvalid)
		}
		// An oversized page is clamped rather than refused (RFC 0014 §4, AIP-158). The clamp
		// lands on a copy so the caller's request is not rewritten through the shared pointer.
		clamped := *query.Pagination
		clamped.PageSize = min(clamped.PageSize, tracestore.MaxPageSize)
		query.Pagination = &clamped
	}
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
		finalized, err := tracestore.FinalizeFilter(query.Filter)
		if err != nil {
			return ctx, query, fmt.Errorf("%w: %w", tracestore.ErrFilterInvalid, err)
		}
		query.Filter = finalized
	}
	if len(qs.options.Interceptors) > 0 {
		var err error
		ctx, query, err = qs.onTraceQuery(ctx, query)
		if err != nil {
			return ctx, query, err
		}
	}
	if query.Filter == nil && query.Pagination == nil {
		return ctx, query, qs.checkServiceName(ctx, query)
	}
	caps := qs.readerSearchCapabilitiesOrDefault(ctx)
	// The filter is settled before the service name is checked, because a filter can name the
	// service itself and rewriting it is what moves that into ServiceName.
	prepared, err := queryToReaderShape(query.TraceQueryParams, caps)
	if err != nil {
		return ctx, query, err
	}
	query.TraceQueryParams = prepared
	if query.ServiceName == "" && !caps.WithoutServiceName {
		return ctx, query, ErrServiceNameRequired
	}
	return ctx, query, nil
}

// normalizeEnvelope checks the fields every trace search carries whichever filtering model it
// uses, and applies DefaultSearchDepth where the caller left the bound unset. The API handlers
// only translate their wire shape into this one; what a query must satisfy is decided here, once.
func (q *TraceQueryParams) normalizeEnvelope() error {
	if q.StartTimeMin.IsZero() || q.StartTimeMax.IsZero() {
		return fmt.Errorf("%w: min and max start time are required", ErrQueryInvalid)
	}
	if !q.StartTimeMin.Before(q.StartTimeMax) {
		return fmt.Errorf("%w: min start time must be before max start time", ErrQueryInvalid)
	}
	if q.DurationMin < 0 || q.DurationMax < 0 {
		return fmt.Errorf("%w: min and max duration cannot be negative", ErrQueryInvalid)
	}
	if q.DurationMin > 0 && q.DurationMax > 0 && q.DurationMax < q.DurationMin {
		return fmt.Errorf("%w: max duration cannot be less than min duration", ErrQueryInvalid)
	}
	if q.SearchDepth < 0 || q.SearchDepth > tracestore.MaxSearchDepth {
		return fmt.Errorf("%w: search depth must be in [0, %d]", ErrQueryInvalid, tracestore.MaxSearchDepth)
	}
	if q.SearchDepth == 0 && q.Pagination == nil {
		q.SearchDepth = DefaultSearchDepth
	}
	return nil
}

// prepareAndInterceptSpanSearchQuery is prepareAndInterceptSearchQuery for a span search (RFC 0016
// §4.6): it refuses a request this deployment or its backend does not accept, runs the configured
// query interceptors (OnSpanQuery) over it, and returns the query to dispatch along with the context
// the interceptors returned. A span query has one shape, so there is no conversion step and no
// service-name rule. The API handlers
// only translate their wire shape into this one; what a query must satisfy is decided here, once
// (the same reasoning as normalizeEnvelope, for the one field a span query's envelope has).
//
// The capabilities are read once. The caller's filter validation and the span-search refusal
// come before the interceptors, so an interceptor is never shown a request this deployment or
// its backend refuses outright; the filter capability check comes after them, so a predicate an
// interceptor adds is held to the same check as one the caller sent.
func (qs QueryService) prepareAndInterceptSpanSearchQuery(
	ctx context.Context,
	query SpanQueryParams,
) (context.Context, SpanQueryParams, error) {
	if query.StartTimeMin.IsZero() || query.StartTimeMax.IsZero() {
		return ctx, query, fmt.Errorf("%w: start_time_min and start_time_max are required", ErrQueryInvalid)
	}
	if !query.StartTimeMin.Before(query.StartTimeMax) {
		return ctx, query, fmt.Errorf("%w: start_time_min must be before start_time_max", ErrQueryInvalid)
	}
	// A page token is what makes this a paginated request, and that is what the feature gate
	// governs. The page size is only the bound (RFC 0016 §6): unset means the default, as an
	// omitted size does on Elasticsearch, and an oversized one is clamped (RFC 0014 §4).
	if len(query.Pagination.PageToken) != 0 && !PaginationGate.IsEnabled() {
		return ctx, query, fmt.Errorf("%w: enable the %q feature gate to use it",
			ErrPaginationDisabled, PaginationGate.ID())
	}
	if query.Pagination.PageSize == 0 {
		query.Pagination.PageSize = DefaultPageSize
	}
	query.Pagination.PageSize = min(query.Pagination.PageSize, tracestore.MaxPageSize)
	// A search over the time range alone carries no filter and is the base case of a span query
	// (RFC 0016 §5.1), so the gate and finalization apply only when the caller sent one. Neither
	// depends on the backend, so both come before the capability call rather than after it.
	if query.Filter != nil {
		if !StructuredFiltersGate.IsEnabled() {
			return ctx, query, fmt.Errorf("%w: enable the %q feature gate to use it",
				ErrFilterDisabled, StructuredFiltersGate.ID())
		}
		finalized, err := tracestore.FinalizeFilter(query.Filter)
		if err != nil {
			return ctx, query, fmt.Errorf("%w: %w", tracestore.ErrFilterInvalid, err)
		}
		query.Filter = finalized
	}
	caps := qs.readerSearchCapabilitiesOrDefault(ctx)
	if !caps.SpanSearch {
		return ctx, query, ErrSpanSearchUnsupported
	}
	if len(qs.options.Interceptors) > 0 {
		var err error
		ctx, query, err = qs.onSpanQuery(ctx, query)
		if err != nil {
			return ctx, query, err
		}
	}
	if err := ensureSpanPaginationSupported(caps, query.Pagination); err != nil {
		return ctx, query, err
	}
	return ctx, query, ensureSpanFilterSupported(caps, query.Filter)
}

// ensureSpanPaginationSupported is RFC 0014 §6.2 for a span search. A reader that cannot paginate
// still serves one page bounded by PageSize, since a span query has no other bound to fold it
// into, but it cannot have minted a PageToken, so a query carrying one is refused rather than
// restarted as a new search.
func ensureSpanPaginationSupported(caps tracestore.SearchCapabilities, pagination tracestore.Pagination) error {
	if len(pagination.PageToken) != 0 && !caps.Paginated {
		return tracestore.ErrPaginationUnsupported
	}
	return nil
}

// readerSearchCapabilitiesOrDefault reads what the reader declares. A reader that cannot report
// its capabilities reads as the least capable one, which serves only the legacy predicate fields
// and no span search.
func (qs QueryService) readerSearchCapabilitiesOrDefault(ctx context.Context) tracestore.SearchCapabilities {
	caps, err := qs.traceReader.SearchCapabilities(ctx)
	if err != nil {
		caps = tracestore.SearchCapabilities{}
	}
	return caps
}

// ensureSpanFilterSupported refuses a filter the reader did not declare it can evaluate. A span
// query has no legacy shape to rewrite an unsupported filter into, so a reader that declared no
// filter support at all refuses every filter rather than receiving it unchecked.
func ensureSpanFilterSupported(caps tracestore.SearchCapabilities, filter *expression.Call) error {
	if filter == nil {
		return nil
	}
	if caps.Filter.IsEmpty() {
		return fmt.Errorf("%w: this storage backend evaluates no filter", tracestore.ErrFilterUnsupported)
	}
	return caps.Filter.EnsureSupported(filter)
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
) iter.Seq2[PageChunk[[]tracestore.TraceSummary], error] {
	return func(yield func(PageChunk[[]tracestore.TraceSummary], error) bool) {
		ctx, query, err := qs.prepareAndInterceptSearchQuery(ctx, query)
		if err != nil {
			yield(PageChunk[[]tracestore.TraceSummary]{}, err)
			return
		}
		// A reader that cannot paginate has had its Pagination cleared by now; one that can is
		// handed the cursor its token wraps.
		minter, err := resumeTraceSearch(&query.TraceQueryParams)
		if err != nil {
			yield(PageChunk[[]tracestore.TraceSummary]{}, err)
			return
		}
		for chunk, err := range qs.traceReader.FindTraceSummaries(ctx, query.TraceQueryParams) {
			if err != nil {
				if errors.Is(err, errors.ErrUnsupported) {
					// Fall back to FindTraces + aggregation. The fallback loads whole traces, so
					// the interceptors get the same say over them as on a FindTraces search; the
					// summaries computed from them carry no spans and have no hook of their own.
					traces := qs.interceptTraceResults(ctx, qs.traceReader.FindTraces(ctx, query.TraceQueryParams))
					for b, e := range computeSummaries(traces, qs.adjuster) {
						// FindTraces does not return pagination metadata, so fallback results cannot
						// supply a next-page token.
						result := PageChunk[[]tracestore.TraceSummary]{
							Results:       b,
							NextPageToken: "",
						}
						if !yield(result, e) {
							return
						}
					}
					return
				}
				yield(PageChunk[[]tracestore.TraceSummary]{}, err)
				return
			}
			result := PageChunk[[]tracestore.TraceSummary]{Results: chunk.Results}
			result.NextPageToken, err = minter.mint(chunk.NextPageToken)
			if !yield(result, err) {
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
	// use primary reader only to avoid readArchive->archive cycle
	getTracesIter := qs.traceReader.GetTraces(ctx, query)
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
