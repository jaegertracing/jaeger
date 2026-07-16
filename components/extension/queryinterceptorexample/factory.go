// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package queryinterceptorexample exposes, for OCB custom builds, a
// demonstration extension that implements the query-interceptor contract.
package queryinterceptorexample

import impl "github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/queryinterceptorexample"

var NewFactory = impl.NewFactory
