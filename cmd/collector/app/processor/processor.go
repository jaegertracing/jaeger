// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package processor

import (
	"context"
	"io"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

var (
	_ Batch = (*SpansV1)(nil)
	_ Batch = (*SpansV2)(nil)
)

// Batch is a batch of spans passed to the processor.
type Batch interface {
	// GetSpans delegates to the appropriate function based on the data model version.
	GetSpans(sv1 func(spans []*model.Span), sv2 func(traces ptrace.Traces))

	GetSpanFormat() SpanFormat
	GetInboundTransport() InboundTransport
	GetTenant() string
}

// SpanProcessor handles spans
type SpanProcessor interface {
	// ProcessSpans processes spans and return with either a list of true/false success or an error
	ProcessSpans(ctx context.Context, spans Batch) ([]bool, error)
	io.Closer
}

type Details struct {
	SpanFormat       SpanFormat
	InboundTransport InboundTransport
	Tenant           string
}

// Spans is a batch of spans passed to the processor.
type SpansV1 struct {
	Spans []*model.Span
	Details
}

type SpansV2 struct {
	Traces ptrace.Traces
	Details
}

func (s SpansV1) GetSpans(sv1 func([]*model.Span), _ func(ptrace.Traces)) {
	sv1(s.Spans)
}

func (s SpansV2) GetSpans(_ func([]*model.Span), v2 func(ptrace.Traces)) {
	v2(s.Traces)
}

func (d Details) GetSpanFormat() SpanFormat {
	return d.SpanFormat
}

func (d Details) GetInboundTransport() InboundTransport {
	return d.InboundTransport
}

func (d Details) GetTenant() string {
	return d.Tenant
}
