// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package processor

import (
	"errors"
	"io"

	"github.com/jaegertracing/jaeger/model"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// ErrBusy signalizes that processor cannot process incoming data
var ErrBusy = errors.New("server busy")

// SpansOptions additional options passed to processor along with the spans.
type Spans struct {
	SpansV1          []*model.Span
	SpansV2          ptrace.Traces
	SpanFormat       SpanFormat
	InboundTransport InboundTransport
	Tenant           string
}

// SpanProcessor handles model spans
type SpanProcessor interface {
	// ProcessSpans processes model spans and return with either a list of true/false success or an error
	ProcessSpans(spans Spans) ([]bool, error)
	io.Closer
}

// InboundTransport identifies the transport used to receive spans.
type InboundTransport string

const (
	// GRPCTransport indicates spans received over gRPC.
	GRPCTransport InboundTransport = "grpc"
	// HTTPTransport indicates spans received over HTTP.
	HTTPTransport InboundTransport = "http"
	// UnknownTransport is the fallback/catch-all category.
	UnknownTransport InboundTransport = "unknown"
)

// SpanFormat identifies the data format in which the span was originally received.
type SpanFormat string

const (
	// JaegerSpanFormat is for Jaeger Thrift spans.
	JaegerSpanFormat SpanFormat = "jaeger"
	// ZipkinSpanFormat is for Zipkin Thrift spans.
	ZipkinSpanFormat SpanFormat = "zipkin"
	// ProtoSpanFormat is for Jaeger protobuf Spans.
	ProtoSpanFormat SpanFormat = "proto"
	// OTLPSpanFormat is for OpenTelemetry OTLP format.
	OTLPSpanFormat SpanFormat = "otlp"
	// UnknownSpanFormat is the fallback/catch-all category.
	UnknownSpanFormat SpanFormat = "unknown"
)
