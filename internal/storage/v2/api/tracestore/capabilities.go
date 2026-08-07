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

// NoSearchCapabilities provides a Reader.SearchCapabilities implementation that
// declares the zero value: every optional query field is required. Embed it in a
// Reader to say so without writing the method by hand.
//
// It is named for the whole set rather than for any one field, because that is what
// embedding it declares — a capability added to SearchCapabilities later is reported
// as unsupported here too, which is the conservative default a backend that has not
// been assessed for it should have.
type NoSearchCapabilities struct{}

func (NoSearchCapabilities) SearchCapabilities() SearchCapabilities {
	return SearchCapabilities{}
}
