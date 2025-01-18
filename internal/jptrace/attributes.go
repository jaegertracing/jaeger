// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
package jptrace

const (
	// WarningsAttribute is the name of the span attribute where we can
	// store various warnings produced from transformations,
	// such as inbound sanitizers and outbound adjusters.
	// The value type of the attribute is a string slice.
	WarningsAttribute = "@jaeger@warnings"
	// FormatAttribute is a key for span attribute that records the original
	// wire format in which the span was received by Jaeger,
	// e.g. proto, thrift, json.
	FormatAttribute = "@jaeger@format"
	// HashAttrivute is the name of the span attribute where we can
	// store hash values of the span . The hash value can be used to
	// skip hash computation and used for deduplication of spans
	HashAttribute = "@jaeger@hash"
)
