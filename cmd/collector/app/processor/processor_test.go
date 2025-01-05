// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
)

func TestDetails(t *testing.T) {
	d := Details{
		SpanFormat:       JaegerSpanFormat,
		InboundTransport: InboundTransport("grpc"),
		Tenant:           "tenant",
	}
	assert.Equal(t, JaegerSpanFormat, d.GetSpanFormat())
	assert.Equal(t, InboundTransport("grpc"), d.GetInboundTransport())
	assert.Equal(t, "tenant", d.GetTenant())
}

func TestSpansV1(t *testing.T) {
	s := SpansV1{
		Spans: []*model.Span{{}},
		Details: Details{
			SpanFormat:       JaegerSpanFormat,
			InboundTransport: InboundTransport("grpc"),
			Tenant:           "tenant",
		},
	}
	var spans []*model.Span
	s.GetSpans(func(s []*model.Span) {
		spans = s
	}, func(_ ptrace.Traces) {
		panic("not implemented")
	})
	assert.Equal(t, []*model.Span{{}}, spans)
	assert.Equal(t, JaegerSpanFormat, s.GetSpanFormat())
	assert.Equal(t, InboundTransport("grpc"), s.GetInboundTransport())
	assert.Equal(t, "tenant", s.GetTenant())
}

func TestSpansV2(t *testing.T) {
	s := SpansV2{
		Traces: ptrace.NewTraces(),
		Details: Details{
			SpanFormat:       JaegerSpanFormat,
			InboundTransport: InboundTransport("grpc"),
			Tenant:           "tenant",
		},
	}
	var traces ptrace.Traces
	s.GetSpans(func(_ []*model.Span) {
		panic("not implemented")
	}, func(t ptrace.Traces) {
		traces = t
	})
	assert.Equal(t, ptrace.NewTraces(), traces)
	assert.Equal(t, JaegerSpanFormat, s.GetSpanFormat())
	assert.Equal(t, InboundTransport("grpc"), s.GetInboundTransport())
	assert.Equal(t, "tenant", s.GetTenant())
}
