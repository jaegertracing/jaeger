// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package queryinterceptor defines the extension point that lets an
// OpenTelemetry extension participate in jaeger-query's read path, without
// exposing the storage Reader itself.
//
// It is the query-side analogue of an authenticator extension: jaeger-query
// resolves the configured interceptor extensions from the collector host (by
// component ID, in order) and invokes them around every trace query. An
// interceptor gets two narrow hooks, matching the two enforcement points a
// read path has:
//
//   - OnQuery runs *before* the search executes, so it can reject a query or
//     constrain it (e.g. add a filter). This is the only point at which a
//     component can stop a query from matching on data the caller may not read.
//   - OnResult runs on each batch of loaded traces *before* it is returned, so
//     it can drop whole traces or redact sub-attributes.
//
// The business logic (authorization, redaction rules, …) lives entirely in the
// extension; jaeger-query only defines the contract and the invocation points.
//
// This package is the implementation. The contract is re-exported for external
// consumers (custom OCB builds, third-party extensions) at the module-root path
// github.com/jaegertracing/jaeger/components/extension/queryinterceptor, which
// aliases Interceptor and the TraceQueryParams type so implementers never need
// to import a Jaeger-internal package — the same way the Collector publishes
// extensionauth for authenticators.
package queryinterceptor

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// Interceptor is implemented by an extension that wants to gate queries and/or
// sanitize results on jaeger-query's read path. An implementation is an
// ordinary component.Component (an OTel extension) that additionally satisfies
// this interface.
type Interceptor interface {
	// OnQuery is invoked before a trace search executes. Returning an error
	// rejects the query (the caller sees that error); returning a modified
	// query constrains what the search may match. Implementations must return
	// the query they want executed — returning the input unchanged is a no-op.
	OnQuery(ctx context.Context, query tracestore.TraceQueryParams) (tracestore.TraceQueryParams, error)

	// OnResult is invoked on each batch of traces before it is returned to the
	// caller. The returned batch replaces the input; an implementation may drop
	// whole traces or redact sub-attributes within them. Returning an error
	// aborts the result stream.
	OnResult(ctx context.Context, traces []ptrace.Traces) ([]ptrace.Traces, error)
}

// Resolve looks up the extensions named by ids on the collector host and
// asserts that each one implements Interceptor, returning them in the order
// given. An empty ids slice yields no interceptors (and no error).
//
// This mirrors the OpenTelemetry Collector's own auth-extension mechanism: a
// component references an extension by component.ID in its config and, at start
// time, resolves it from host.GetExtensions() and asserts it implements the
// expected interface — extensionauth.Server (via configauth) for authenticators,
// Interceptor here. (It is not Jaeger's internal bearer-token/tenancy auth.)
func Resolve(host component.Host, ids []component.ID) ([]Interceptor, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	extensions := host.GetExtensions()
	interceptors := make([]Interceptor, 0, len(ids))
	for _, id := range ids {
		comp, ok := extensions[id]
		if !ok {
			return nil, fmt.Errorf(
				"cannot find query interceptor extension %q (make sure it is defined and listed earlier in the config)",
				id,
			)
		}
		interceptor, ok := comp.(Interceptor)
		if !ok {
			return nil, fmt.Errorf("extension %q does not implement queryinterceptor.Interceptor", id)
		}
		interceptors = append(interceptors, interceptor)
	}
	return interceptors, nil
}
