// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

// SearchCapabilities reports which normally-required TraceQueryParams fields a
// Reader's search methods accept as empty (RFC 0013 §3.1). Its zero value declares
// that every field is required, so a reader that gains no capability needs no change
// when a field is added here.
type SearchCapabilities struct {
	// WithoutServiceName is true when FindTraces, FindTraceIDs and FindTraceSummaries
	// accept a TraceQueryParams whose ServiceName is empty and read it as "any
	// service", rather than as an error or an empty result.
	WithoutServiceName bool
}
