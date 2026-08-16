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
// The types here depend only on public packages (OTel pdata), so custom OCB
// builds and third-party extensions implement this contract without importing
// any jaeger-internal package. Query is a stable, purpose-built view: it is
// deliberately decoupled from jaeger-query's internal query struct so the
// internals can evolve without breaking this contract.
package queryinterceptor

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Query is the public view of a trace-search query passed to Interceptor.OnQuery.
type Query struct {
	ServiceName   string
	OperationName string
	// Attributes holds the tag/attribute filters of the query. When building a
	// Query, initialize it with pcommon.NewMap().
	Attributes   pcommon.Map
	StartTimeMin time.Time
	StartTimeMax time.Time
	DurationMin  time.Duration
	DurationMax  time.Duration
	SearchDepth  int
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
