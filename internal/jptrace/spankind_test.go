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
