// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package queryinterceptor wires the public query-interceptor contract into
// jaeger-query's read path: it resolves the configured interceptor extensions
// from the collector host and decorates the storage Reader with them, converting
// between the internal query type and the public Query at the boundary.
//
// The public contract that extensions implement — the Interceptor interface and
// the Query type — lives at
// github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor
// and depends only on public packages. This package is jaeger-query-private
// machinery and is not part of that contract.
package queryinterceptor

import (
	"fmt"

	"go.opentelemetry.io/collector/component"

	pub "github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
)

// Resolve looks up the extensions named by ids on the collector host and asserts
// that each implements the public Interceptor contract, returning them in order.
// An empty ids slice yields no interceptors (and no error).
//
// This mirrors the OpenTelemetry Collector's auth-extension mechanism: a
// component references an extension by component.ID in its config and resolves
// it from host.GetExtensions() at start time.
func Resolve(host component.Host, ids []component.ID) ([]pub.Interceptor, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	extensions := host.GetExtensions()
	interceptors := make([]pub.Interceptor, 0, len(ids))
	for _, id := range ids {
		comp, ok := extensions[id]
		if !ok {
			return nil, fmt.Errorf(
				"cannot find query interceptor extension %q (make sure it is defined under extensions: and enabled in service.extensions)",
				id,
			)
		}
		interceptor, ok := comp.(pub.Interceptor)
		if !ok {
			return nil, fmt.Errorf("extension %q does not implement queryinterceptor.Interceptor", id)
		}
		interceptors = append(interceptors, interceptor)
	}
	return interceptors, nil
}
