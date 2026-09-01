// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"iter"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// Reader finds and loads traces and other data from storage.
//
// Ownership of returned traces: the caller owns every ptrace.Traces a Reader
// yields and may modify it in place — query-time adjusters do, and so does any
// query-interceptor extension that rewrites spans on the return path. An
// implementation that keeps its own copy of the data (an in-memory backend, or
// any backend with a read cache) MUST therefore yield a deep copy rather than a
// reference to what it holds, or a single reader will corrupt the stored trace
// for every later one.
type Reader interface {
	// GetTraces returns an iterator that retrieves all traces with given IDs.
	// The iterator is single-use: once consumed, it cannot be used again.
	//
	// Chunking requirements:
	// - A single ptrace.Traces chunk MUST NOT contain spans from multiple traces.
	// - Large traces MAY be split across multiple, *consecutive* ptrace.Traces chunks.
	// - Each returned ptrace.Traces object MUST NOT be empty.
	//
	// Edge cases:
	// - If no spans are found for any given trace ID, the ID is ignored.
	// - If none of the trace IDs are found in the storage, an empty iterator is returned.
	// - If an error is encountered, the iterator returns the error and stops.
	GetTraces(ctx context.Context, traceIDs ...GetTraceParams) iter.Seq2[[]ptrace.Traces, error]

	// GetServices returns all service names known to the backend from spans
	// within its retention period.
	GetServices(ctx context.Context) ([]string, error)

	// GetOperations returns all operation names for a given service
	// known to the backend from spans within its retention period.
	GetOperations(ctx context.Context, query OperationQueryParams) ([]Operation, error)

	// FindTraces returns an iterator that retrieves traces matching query parameters.
	// The iterator is single-use: once consumed, it cannot be used again.
	//
	// The chunking rules is the same as for GetTraces.
	//
	// If no matching traces are found, the function returns an empty iterator.
	// If an error is encountered, the iterator returns the error and stops.
	//
	// There's currently an implementation-dependent ambiguity whether all query filters
	// (such as multiple tags) must apply to the same span within a trace, or can be satisfied
	// by different spans.
	FindTraces(ctx context.Context, query TraceQueryParams) iter.Seq2[[]ptrace.Traces, error]

	// FindTraceIDs returns an iterator that retrieves IDs of traces matching query parameters.
	// The iterator is single-use: once consumed, it cannot be used again.
	//
	// If no matching traces are found, the function returns an empty iterator.
	// If an error is encountered, the iterator returns the error and stops.
	//
	// This function behaves identically to FindTraces, except that it returns only the list
	// of matching trace IDs. This is useful in some contexts, such as batch jobs, where a
	// large list of trace IDs may be queried first and then the full traces are loaded
	// in batches.
	FindTraceIDs(ctx context.Context, query TraceQueryParams) iter.Seq2[[]FoundTraceID, error]

	// FindTraceSummaries returns an iterator over lightweight summaries of the traces
	// matching the query parameters (the metadata shown in search-result lists). The
	// iterator is single-use: once consumed, it cannot be used again.
	//
	// Backends that can compute summaries natively (e.g. via a storage-side aggregation)
	// should do so. Backends that cannot must yield errors.ErrUnsupported (wrapped with
	// %w) as the first error, before any batch; the caller then falls back to FindTraces
	// plus client-side aggregation. Such backends can embed UnsupportedTraceSummaries to
	// get this behavior for free.
	//
	// The iterator streams result batches; each yielded batch may contain one or more
	// summaries, and implementations may yield incrementally rather than buffering all
	// results first.
	FindTraceSummaries(ctx context.Context, query TraceQueryParams) iter.Seq2[[]TraceSummary, error]

	// SearchCapabilities reports how this reader's search methods behave; see
	// SearchCapabilities for what it describes.
	//
	// A reader that cannot determine its own — one whose backend sits behind an API that
	// cannot be asked — returns errors.ErrUnsupported (wrapped with %w) rather than a
	// value a caller might trust.
	//
	// A Reader that wraps another must forward the call, or it reports the wrapper's
	// capabilities instead of the backend's.
	SearchCapabilities(ctx context.Context) (SearchCapabilities, error)
}

// GetTraceParams contains single-trace parameters for a GetTraces request.
// Some storage backends (e.g. Tempo) perform GetTraces much more efficiently
// if they know the approximate time range of the trace.
type GetTraceParams struct {
	// TraceID is the ID of the trace to retrieve. Required.
	TraceID pcommon.TraceID
	// Start of the time interval to search for trace ID. Optional.
	Start time.Time
	// End of the time interval to search for trace ID. Optional.
	End time.Time
}

// TraceQueryParams contains query parameters to find traces. For a detailed
// definition of each field in this message, refer to `TraceQueryParameters` in `jaeger.api_v3`
// (https://github.com/jaegertracing/jaeger-idl/blob/main/proto/api_v3/query_service.proto).
type TraceQueryParams struct {
	ServiceName   string
	OperationName string
	// Attributes must initialized with pcommon.NewMap() before use.
	Attributes   pcommon.Map
	StartTimeMin time.Time
	StartTimeMax time.Time
	DurationMin  time.Duration
	DurationMax  time.Duration
	SearchDepth  int
	// Filter is the structured query filter (RFC 0005): a boolean-valued Call over
	// level-qualified attributes and built-in fields. It is mutually exclusive with the
	// predicate fields above — ServiceName, OperationName, Attributes and the duration
	// bounds — so a reader sees one filtering model, not a mix of the two. A reader only
	// receives a Filter whose levels and operators its SearchCapabilities declare; for any
	// other reader the query service expresses the filter in the legacy fields instead, or
	// refuses the query.
	Filter *expression.Call
	// Pagination requests a paginated search (RFC 0014). Its zero value means this is not
	// a paginated request, the same as an absent jaeger.api_v3.Pagination on the wire.
	Pagination Pagination
}

// Pagination asks for one page of a search result and, on continuation, says where the
// previous page stopped. It mirrors jaeger.api_v3.Pagination and jaeger.storage.v2.Pagination
// (RFC 0014 §4, §6).
type Pagination struct {
	// PageSize bounds the number of results in one page. Zero means this is not a
	// paginated request; the query service falls back to SearchDepth as the page bound
	// before a Reader ever sees the query (RFC 0014 §4).
	PageSize int
	// PageToken continues a previous search. Empty starts a new one. A Reader that
	// receives a non-empty PageToken MUST treat it as an uninterpreted cursor it minted
	// itself for the same query — a Reader is never asked to interpret a token it did not
	// produce, since the query service rejects a PageToken against a Reader whose
	// SearchCapabilities.Paginated is false before dispatching (RFC 0014 §6.2).
	PageToken string
}

// FoundTraceID is a wrapper around trace ID returned from FindTraceIDs
// with an optional time range that may be used in GetTraces calls.
//
// The time range is provided as an optimization hint for some storage backends
// that can perform more efficient queries when they know the approximate time range.
// The value should not be used for precise time-based filtering or assumptions.
// It is meant as a rough boundary and may not be populated in all cases.
type FoundTraceID struct {
	TraceID pcommon.TraceID
	Start   time.Time
	End     time.Time
}

// OperationQueryParams contains parameters of query operations, empty spanKind means get operations for all kinds of span.
type OperationQueryParams struct {
	ServiceName string
	SpanKind    string
}

// Operation contains operation name and span kind
type Operation struct {
	Name     string
	SpanKind string
}
