// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestStringToSpanKind(t *testing.T) {
	tests := []struct {
		str  string
		want ptrace.SpanKind
	}{
		{
			str:  "Unspecified",
			want: ptrace.SpanKindUnspecified,
		},
		{
			str:  "Internal",
			want: ptrace.SpanKindInternal,
		},
		{
			str:  "Server",
			want: ptrace.SpanKindServer,
		},
		{
			str:  "Client",
			want: ptrace.SpanKindClient,
		},
		{
			str:  "Producer",
			want: ptrace.SpanKindProducer,
		},
		{
			str:  "Consumer",
			want: ptrace.SpanKindConsumer,
		},
		{
			str:  "Unknown",
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
			want: "unspecified",
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
