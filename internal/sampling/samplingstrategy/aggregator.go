// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package samplingstrategy

import (
	"io"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// Aggregator defines an interface used to aggregate operation throughput.
type Aggregator interface {
	// Close() from io.Closer stops the aggregator from aggregating throughput.
	io.Closer

	// The HandleRootSpan function processes a span, checking if it's a root span.
	// If it is, it records the throughput.
	HandleRootSpan(span *model.Span)

	// RecordThroughput records throughput for an operation for aggregation.
	RecordThroughput(service, operation string, samplerType model.SamplerType, probability float64)

	// Start starts aggregating operation throughput.
	Start()
}
