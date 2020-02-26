// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package processor

import (
	"io"

	"github.com/jaegertracing/jaeger/model"
)

// SpansOptions additional options passed to processor along with the spans.
type SpansOptions struct {
	SpanFormat       SpanFormat
	InboundTransport InboundTransport
}

// SpanProcessor handles model spans
type SpanProcessor interface {
	// ProcessSpans processes model spans and return with either a list of true/false success or an error
	ProcessSpans(mSpans []*model.Span, options SpansOptions) ([]bool, error)
	io.Closer
}

// InboundTransport identifies the transport used to receive spans.
type InboundTransport string

const (
	// GRPCTransport indicates spans received over gRPC.
	GRPCTransport InboundTransport = "grpc"
	// TChannelTransport indicates spans received over TChannel.
	TChannelTransport InboundTransport = "tchannel"
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
	// UnknownSpanFormat is the fallback/catch-all category.
	UnknownSpanFormat SpanFormat = "unknown"
)
