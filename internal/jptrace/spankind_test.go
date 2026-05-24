// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/proto-gen/api_v2/metrics"
)

func TestStringToSpanKind(t *testing.T) {
	tests := []struct {
		str  string
		want ptrace.SpanKind
	}{
		{
			str:  "unspecified",
			want: ptrace.SpanKindUnspecified,
		},
		{
			str:  "internal",
			want: ptrace.SpanKindInternal,
		},
		{
			str:  "server",
			want: ptrace.SpanKindServer,
		},
		{
			str:  "client",
			want: ptrace.SpanKindClient,
		},
		{
			str:  "producer",
			want: ptrace.SpanKindProducer,
		},
		{
			str:  "consumer",
			want: ptrace.SpanKindConsumer,
		},
		{
			str:  "unknown",
			want: ptrace.SpanKindUnspecified,
		},
		{
			str:  "",
			want: ptrace.SpanKindUnspecified,
		},
		{
			str:  "invalid",
			want: ptrace.SpanKindUnspecified,
		},
	}
	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			require.Equal(t, tt.want, StringToSpanKind(tt.str))
		})
	}
}

func TestSpanKindToString(t *testing.T) {
	tests := []struct {
		kind ptrace.SpanKind
		want string
	}{
		{
			kind: ptrace.SpanKindUnspecified,
			want: "",
		},
		{
			kind: ptrace.SpanKindInternal,
			want: "internal",
		},
		{
			kind: ptrace.SpanKindServer,
			want: "server",
		},
		{
			kind: ptrace.SpanKindClient,
			want: "client",
		},
		{
			kind: ptrace.SpanKindProducer,
			want: "producer",
		},
		{
			kind: ptrace.SpanKindConsumer,
			want: "consumer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, SpanKindToString(tt.kind))
		})
	}
}

func TestProtoSpanKindToString(t *testing.T) {
	tests := []struct {
		proto string
		want  string
	}{
		{proto: metrics.SpanKind_SPAN_KIND_UNSPECIFIED.String(), want: "unspecified"},
		{proto: metrics.SpanKind_SPAN_KIND_INTERNAL.String(), want: "internal"},
		{proto: metrics.SpanKind_SPAN_KIND_SERVER.String(), want: "server"},
		{proto: metrics.SpanKind_SPAN_KIND_CLIENT.String(), want: "client"},
		{proto: metrics.SpanKind_SPAN_KIND_PRODUCER.String(), want: "producer"},
		{proto: metrics.SpanKind_SPAN_KIND_CONSUMER.String(), want: "consumer"},
		{proto: "span_kind_server", want: "server"},
		{proto: "Span_Kind_Client", want: "client"},
		{proto: "", want: ""},
		{proto: "invalid", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.proto, func(t *testing.T) {
			require.Equal(t, tt.want, ProtoSpanKindToString(tt.proto))
		})
	}
}

func TestStringToProtoSpanKind(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "unspecified", want: metrics.SpanKind_SPAN_KIND_UNSPECIFIED.String()},
		{input: "internal", want: metrics.SpanKind_SPAN_KIND_INTERNAL.String()},
		{input: "server", want: metrics.SpanKind_SPAN_KIND_SERVER.String()},
		{input: "client", want: metrics.SpanKind_SPAN_KIND_CLIENT.String()},
		{input: "producer", want: metrics.SpanKind_SPAN_KIND_PRODUCER.String()},
		{input: "consumer", want: metrics.SpanKind_SPAN_KIND_CONSUMER.String()},
		{input: "Server", want: metrics.SpanKind_SPAN_KIND_SERVER.String()},
		{input: "CLIENT", want: metrics.SpanKind_SPAN_KIND_CLIENT.String()},
		{input: "unknown", want: ""},
		{input: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.want, StringToProtoSpanKind(tt.input))
		})
	}
}

func TestProtoSpanKindRoundTrip(t *testing.T) {
	kinds := []string{
		"unspecified",
		"internal",
		"server",
		"client",
		"producer",
		"consumer",
	}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			proto := StringToProtoSpanKind(kind)
			require.NotEmpty(t, proto)
			require.Equal(t, kind, ProtoSpanKindToString(proto))
		})
	}
}

func TestSpanKindRoundTrip(t *testing.T) {
	kinds := []string{
		"internal",
		"server",
		"client",
		"producer",
		"consumer",
	}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			spanKind := StringToSpanKind(kind)
			require.Equal(t, kind, SpanKindToString(spanKind))
		})
	}
}
