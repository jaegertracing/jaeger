// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestConvertStatusCode(t *testing.T) {
	type args struct {
		sc string
	}
	tests := []struct {
		str  string
		want ptrace.StatusCode
	}{
		{
			str:  "Ok",
			want: ptrace.StatusCodeOk,
		},
		{
			str:  "Unset",
			want: ptrace.StatusCodeUnset,
		},
		{
			str:  "Error",
			want: ptrace.StatusCodeError,
		},
		{
			str:  "Unknown",
			want: ptrace.StatusCodeUnset,
		},
		{
			str:  "",
			want: ptrace.StatusCodeUnset,
		},
		{
			str:  "invalid",
			want: ptrace.StatusCodeUnset,
		},
	}
	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			require.Equal(t, tt.want, StringToStatusCode(tt.str))
		})
	}
}
