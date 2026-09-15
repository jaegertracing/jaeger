// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package queryinterceptor defines the extension contract that lets an
// OpenTelemetry extension participate in jaeger-query's read path — without
// exposing jaeger-query's storage Reader or its internal query representation.
//
// Motivation: sensitive traces — GenAI model prompts and completions, tool-call
// payloads, PII — must be shown or withheld per user. An interceptor lets a
// deployment enforce that policy at query time, integrating with an in-house
// access-control system that cannot live in open-source Jaeger. OnQuery can
// reject or scope a search so it cannot match on data the caller may not read
// (e.g. a full-text search over prompt content); OnResult can drop whole traces
// or mask sub-attributes on the way out (e.g. redacting PII fields for callers
// without clearance). See the runnable example extension at
// github.com/jaegertracing/jaeger/components/extension/queryinterceptorexample.
//
// It is the query-side analogue of the Collector's authenticator extensions:
// jaeger-query resolves the configured interceptor extensions from the host by
// component ID and invokes them around every trace query. OnQuery runs before
// the search (to reject or constrain it); OnResult runs on each batch of loaded
// traces before it is returned (to drop or redact them). The business logic —
// authorization, redaction — lives entirely in the extension.
//
// The types here depend only on public packages (OTel pdata, and the filter AST from
// jaeger-idl), so custom OCB builds and third-party extensions implement this contract
// without importing any jaeger-internal package. Query is a purpose-built view of a search
// rather than jaeger-query's internal query struct, so most of what that struct changes is
// invisible here. Filter is the exception: it is the same AST the internal query and the
// storage protocol carry, so a change to the AST is a change to this contract — which is why
// the AST lives in a public, versioned module.
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

// Query is the public view of a trace-search query passed to Interceptor.OnQuery.
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
// remaining fields are the envelope, which no predicate lives in.
type Query struct {
	// Filter is the query's predicates as a boolean-valued expression (RFC 0005 §6), or nil
	// when the search asks for a time range and nothing else. Nil rather than an empty
	// conjunction, because `and` takes two arguments or more, so there is no expression that
	// says "match everything".
	//
	// Scoping a query means narrowing this — replacing it, or conjoining a predicate the caller
	// is permitted — or refusing the query outright by returning an error. Returning nil is not
	// a way to decline: it asks for every trace in the time range, and is refused as invalid if
	// the query had predicates when OnQuery received it.
	//
	// jaeger-query decides how to send the search to storage only after the interceptors have
	// finished with it, so a filter naming a level or an operator the storage backend cannot
	// serve is refused on the same terms whether the caller wrote that predicate or an
	// interceptor added it.
	Filter *expression.Call

	StartTimeMin time.Time
	StartTimeMax time.Time
	SearchDepth  int

	// PageSize and PageToken mirror jaeger-query's internal Pagination (RFC 0014) — not
	// imported here, since this package depends only on public packages, so the two fields sit
	// flat on Query instead of nesting a jaeger-internal type. PageSize is zero and PageToken is
	// empty when the search is not paginated, the same as an absent Pagination on the wire.
	// jaeger-query re-validates them against SearchDepth after OnQuery returns, the same as an
	// interceptor's own Filter is finalized on the way back (RFC 0014 §4).
	PageSize  int
	PageToken string
}

// Interceptor is implemented by an extension that gates trace queries and/or
// sanitizes results on jaeger-query's read path. An implementation is an
// ordinary component.Component (an OTel extension) that also satisfies this
// interface, referenced from jaeger_query's query_interceptors config.
//
// Both methods receive the inbound request's context, which is how an
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
// Both methods also *return* a context. jaeger-query threads OnQuery's returned
// context into the storage reader and into OnResult, and threads OnResult's
// returned context into the OnResult call for the next batch of a multi-batch
// result. This lets an implementation do expensive per-query work once — resolve
// the caller's identity against a policy system in OnQuery — and stash the result
// (via context.WithValue) for the return path to reuse, rather than repeating it
// on every batch. Return the inbound context unchanged when there is nothing to
// carry across.
type Interceptor interface {
	// OnQuery runs before a trace search executes. Returning an error rejects
	// the query (the caller sees the error); returning a modified Query
	// constrains what the search may match. The returned context is threaded into
	// the storage reader and OnResult. Return the inbound context and query
	// unchanged for a no-op.
	OnQuery(ctx context.Context, query Query) (context.Context, Query, error)

	// OnResult runs on each batch of traces before it is returned to the caller.
	// The returned batch replaces the input; an implementation may drop whole
	// traces or redact sub-attributes. The returned context is threaded into the
	// OnResult call for the next batch, so state can accumulate across a
	// multi-batch result. Returning an error aborts the stream. Return the inbound
	// context and traces unchanged for a no-op.
	OnResult(ctx context.Context, traces []ptrace.Traces) (context.Context, []ptrace.Traces, error)
}
