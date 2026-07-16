// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package queryinterceptor exposes the jaeger-query query-interceptor extension
// contract for custom Jaeger builds (OCB) and third-party extensions. An
// extension implements Interceptor to gate trace queries (OnQuery) and sanitize
// results (OnResult); jaeger_query invokes it via its query_interceptors config.
package queryinterceptor

import impl "github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/queryinterceptor"

// Interceptor is the contract a query-interceptor extension implements.
type Interceptor = impl.Interceptor

// TraceQueryParams is the query passed to Interceptor.OnQuery.
type TraceQueryParams = impl.TraceQueryParams
