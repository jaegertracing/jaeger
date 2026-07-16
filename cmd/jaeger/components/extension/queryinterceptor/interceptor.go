// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package queryinterceptor re-exports the jaeger-query query-interceptor
// extension contract. This bridge lives under cmd/jaeger (so it may import the
// implementation, which is Jaeger-internal) and is itself re-exported from the
// module-root components/extension/queryinterceptor for OCB custom builds and
// third-party extensions.
package queryinterceptor

import (
	impl "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// Interceptor is the contract a query-interceptor extension implements. See the
// implementation package for the OnQuery/OnResult semantics.
type Interceptor = impl.Interceptor

// TraceQueryParams is the query passed to Interceptor.OnQuery, re-exported so
// implementers can name it through this package instead of an internal one.
type TraceQueryParams = tracestore.TraceQueryParams
