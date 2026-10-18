// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package queryinterceptor defines the extension contract that lets an
// OpenTelemetry extension participate in jaeger-query's read path — without
// exposing jaeger-query's storage Reader or its internal query representation.
//
// Motivation: sensitive traces — GenAI model prompts and completions, tool-call
// payloads, PII — must be shown or withheld per user. An interceptor lets a
// deployment enforce that policy at query time, integrating with an in-house
// access-control system that cannot live in open-source Jaeger. OnTraceQuery can
// reject or scope a search so it cannot match on data the caller may not read
// (e.g. a full-text search over prompt content); OnTraceResult can drop whole traces
// or mask sub-attributes on the way out (e.g. redacting PII fields for callers
// without clearance). See the runnable example extension at
// github.com/jaegertracing/jaeger/components/extension/queryinterceptorexample.
//
// It is the query-side analogue of the Collector's authenticator extensions:
// jaeger-query resolves the configured interceptor extensions from the host by
// component ID and invokes them on the read path. OnTraceQuery runs before every
// trace search, FindTraces and FindTraceSummaries alike, to reject or constrain it.
// OnTraceResult runs on every batch of whole traces before it is returned: the
// batches of FindTraces and GetTraces, and of the FindTraces fallback that serves
// FindTraceSummaries when the reader cannot compute summaries natively. A summary
// the reader computes itself carries no trace and does not pass through it, so a
// policy that must hold on summaries has to be expressed in OnTraceQuery. A span
// search (FindSpans, RFC 0016) runs through OnSpanQuery and OnSpanResult the same
// way. The business logic — authorization, redaction — lives entirely in the
// extension.
//
// The types here depend only on public packages (OTel pdata, and the filter AST from
// jaeger-idl), so custom OCB builds and third-party extensions implement this contract
// without importing any jaeger-internal package. TraceQuery and SpanQuery are purpose-built
// views of a search rather than jaeger-query's internal query structs, so most of what those
// structs change is invisible here. Filter is the exception: it is the same AST the internal
// query and the storage protocol carry, so a change to the AST is a change to this contract —
// which is why the AST lives in a public, versioned module.
package queryinterceptor

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// ErrAccessDenied is the sentinel that interceptor implementations wrap
// when the caller's query is refused on access-control grounds. The API
// layers map it to HTTP 403 / gRPC PERMISSION_DENIED instead of a
// generic server error.
var ErrAccessDenied = errors.New("access denied")

// ErrSpanSearchUnsupported is returned by UnsupportedSpanSearch, and may be returned by any
// implementation, to refuse a span search this interceptor does not gate. jaeger-query
// refuses the search rather than running it past an interceptor that never saw it.
var ErrSpanSearchUnsupported = errors.New("query interceptor does not support span search")

// TraceQuery is the view of a trace search passed to Interceptor.OnTraceQuery.
//
// EXPERIMENTAL: this type and the Interceptor contract it belongs to may change or be removed
// in any release, without a deprecation period. Filter in particular is an RFC 0005 filter AST,
// which that RFC is still moving through its milestones, so an implementation should expect to
// be updated alongside jaeger-query rather than to keep compiling against a stable shape.
//
// Every predicate is in Filter, including the ones a caller sent as the older scalar search
// fields: jaeger-query expresses a service, an operation name, a tag and a duration bound as
// filter predicates before an interceptor sees them, so an implementation reads and rewrites
// one thing rather than a filter plus four fields that can say the same in two ways. The
// remaining fields are the envelope, which no predicate lives in. The result bound (search
// depth or page size) is not part of the view: it selects how much of the result to return,
// not which data may be read, so an interceptor has no say over it.
type TraceQuery struct {
	// Filter is the query's predicates as a boolean-valued expression (RFC 0005 §6), or nil
	// when the search asks for a time range and nothing else. Nil rather than an empty
	// conjunction, because `and` takes two arguments or more, so there is no expression that
	// says "match everything".
	//
	// Scoping a query means narrowing this — replacing it, or conjoining a predicate the caller
	// is permitted — or refusing the query outright by returning an error. Returning nil is not
	// a way to decline: it asks for every trace in the time range, and is refused as invalid if
	// the query had predicates when OnTraceQuery received it.
	//
	// jaeger-query decides how to send the search to storage only after the interceptors have
	// finished with it, so a filter naming a level or an operator the storage backend cannot
	// serve is refused on the same terms whether the caller wrote that predicate or an
	// interceptor added it.
	//
	// A page token is bound to the query as it leaves the interceptors (RFC 0014 §3.2), so a
	// rewrite has to be a pure function of the request it is given: a predicate that changes
	// between two identical requests, such as a bound derived from the current time, makes
	// every continuation look like a different query and be refused.
	Filter *expression.Call

	StartTimeMin time.Time
	StartTimeMax time.Time
}

// SpanQuery is the view of a span search (RFC 0016) passed to Interceptor.OnSpanQuery.
//
// EXPERIMENTAL: see TraceQuery.
//
// It is a separate type from TraceQuery, although the two carry the same fields today,
// because the two searches are expected to diverge. RFC 0016 §5 reserves a projection on the
// span search, which FindSpans can serve as spans with only the selected fields populated,
// and a trace search has no counterpart for it. An interceptor that gates what a caller may
// read has to see such a clause when it arrives, and it belongs on this type alone.
//
// A span search is paginated from its first release, so the rule TraceQuery.Filter states for
// a page token applies to every field here: a rewrite of the filter or of the time range has
// to be a pure function of the request, or no continuation of the search will be accepted.
type SpanQuery struct {
	// Filter has the meaning TraceQuery.Filter documents, over spans rather than traces:
	// nil asks for every span in the time range, and an interceptor that returns nil for a
	// query that had predicates is refused.
	Filter *expression.Call

	StartTimeMin time.Time
	StartTimeMax time.Time
}

// Interceptor is implemented by an extension that gates searches and/or
// sanitizes results on jaeger-query's read path. An implementation is an
// ordinary component.Component (an OTel extension) that also satisfies this
// interface, referenced from jaeger_query's query_interceptors config.
//
// Every method receives the inbound request's context, which is how an
// implementation learns *who* is asking so it can decide per caller. jaeger-query
// runs the request through the Collector's confighttp/configgrpc server, so when
// that server is configured with include_metadata: true the incoming request
// headers are exposed as OTel client metadata:
//
//	role := client.FromContext(ctx).Metadata.Get("x-caller-identity")
//
// (client is go.opentelemetry.io/collector/client). An access-control
// implementation reads the caller's identity/token this way and resolves it
// against its policy system. The example extension does exactly this.
//
// Every method also *returns* a context. jaeger-query threads a pre-query hook's
// returned context into the storage reader and into the matching result hook, and
// threads a result hook's returned context into its call for the next batch or
// chunk of a streamed result. This lets an implementation do expensive per-query
// work once — resolve the caller's identity against a policy system in the
// pre-query hook — and stash the result (via context.WithValue) for the return
// path to reuse, rather than repeating it on every batch. Return the inbound
// context unchanged when there is nothing to carry across.
//
// An implementation that has no policy for span searches embeds UnsupportedSpanSearch,
// which refuses them. Leaving a search kind ungated is not an option this contract offers:
// a policy enforced on FindTraces and absent on FindSpans is a policy a caller can bypass by
// choosing the other endpoint.
type Interceptor interface {
	// OnTraceQuery runs before a trace search executes. Returning an error rejects
	// the query (the caller sees the error); returning a modified TraceQuery
	// constrains what the search may match. The returned context is threaded into
	// the storage reader and OnTraceResult. Return the inbound context and query
	// unchanged for a no-op.
	OnTraceQuery(ctx context.Context, query TraceQuery) (context.Context, TraceQuery, error)

	// OnTraceResult runs on each batch of traces before it is returned to the caller.
	// The returned batch replaces the input; an implementation may drop whole
	// traces or redact sub-attributes. The returned context is threaded into the
	// OnTraceResult call for the next batch, so state can accumulate across a
	// multi-batch result. Returning an error aborts the stream. Return the inbound
	// context and traces unchanged for a no-op.
	OnTraceResult(ctx context.Context, traces []ptrace.Traces) (context.Context, []ptrace.Traces, error)

	// OnSpanQuery runs before a span search (RFC 0016) executes, with the same
	// contract as OnTraceQuery: an error rejects the search, a modified SpanQuery
	// constrains it, and the returned context is threaded into the storage reader
	// and OnSpanResult.
	OnSpanQuery(ctx context.Context, query SpanQuery) (context.Context, SpanQuery, error)

	// OnSpanResult runs on each chunk of a span search's result before it is
	// returned to the caller. A page of results may be streamed as several
	// chunks, so one call does not see a whole page. Unlike a batch of traces, a
	// chunk holds spans from many traces in one ptrace.Traces, so an
	// implementation drops or redacts spans rather than traces. The returned
	// chunk replaces the input; the returned context is threaded into the
	// OnSpanResult call for the next chunk. Returning an error aborts the stream.
	// Return the inbound context and chunk unchanged for a no-op.
	OnSpanResult(ctx context.Context, spans ptrace.Traces) (context.Context, ptrace.Traces, error)
}

// UnsupportedSpanSearch provides the span-search hooks for an Interceptor that has no policy
// for span searches. Both hooks return ErrSpanSearchUnsupported, so jaeger-query refuses a span
// search instead of running it ungated. Embed it in an implementation that gates trace searches
// only:
//
//	type myInterceptor struct {
//		queryinterceptor.UnsupportedSpanSearch
//	}
type UnsupportedSpanSearch struct{}

func (UnsupportedSpanSearch) OnSpanQuery(ctx context.Context, query SpanQuery) (context.Context, SpanQuery, error) {
	return ctx, query, ErrSpanSearchUnsupported
}

func (UnsupportedSpanSearch) OnSpanResult(ctx context.Context, spans ptrace.Traces) (context.Context, ptrace.Traces, error) {
	return ctx, spans, ErrSpanSearchUnsupported
}
