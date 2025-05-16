// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestTODBModel(t *testing.T) {
	trace := jsonToPtrace(t, "./fixtures/ptrace.json")
	actual := ToDBModel(trace)[0]

	expected := jsonToDBModel(t, "./fixtures/dbmodel.json")

	t.Run("Resource", func(t *testing.T) {
		expectedResource := expected.Resource
		actualResource := actual.Resource
		CompareAttributes(t, expectedResource.Attributes, actualResource.Attributes)
	})

	t.Run("Scope", func(t *testing.T) {
		expectedScope := expected.Scope
		actualScope := actual.Scope
		require.Equal(t, expectedScope.Name, actualScope.Name)
		require.Equal(t, expectedScope.Version, actualScope.Version)
		CompareAttributes(t, expectedScope.Attributes, actualScope.Attributes)
	})

	t.Run("Span", func(t *testing.T) {
		expectedSpan := expected.Span
		actualSpan := actual.Span

		require.Equal(t, expectedSpan.Timestamp, actualSpan.Timestamp)
		require.Equal(t, expectedSpan.TraceId, actualSpan.TraceId)
		require.Equal(t, expectedSpan.SpanId, actualSpan.SpanId)
		require.Equal(t, expectedSpan.ParentSpanId, actualSpan.ParentSpanId)
		require.Equal(t, expectedSpan.Name, actualSpan.Name)
		require.Equal(t, expectedSpan.Kind, actualSpan.Kind)
		require.Equal(t, expectedSpan.Duration, actualSpan.Duration)
		require.Equal(t, expectedSpan.StatusCode, actualSpan.StatusCode)
		require.Equal(t, expectedSpan.StatusMessage, actualSpan.StatusMessage)
		CompareAttributes(t, expected.Span.Attributes, actual.Span.Attributes)
	})

	t.Run("Events", func(t *testing.T) {
		expectedEvents := expected.Events
		actualEvents := actual.Events

		for i := range expectedEvents {
			expectedEvent := expectedEvents[i]
			actualEvent := actualEvents[i]

			require.Equal(t, expectedEvent.Name, actualEvent.Name)
			require.Equal(t, expectedEvent.Timestamp, actualEvent.Timestamp)
			CompareAttributes(t, expectedEvent.Attributes, actualEvent.Attributes)
		}
	})

	t.Run("Links", func(t *testing.T) {
		expectedLinks := expected.Links
		actualLinks := actual.Links
		for i := range expectedLinks {
			expectedLink := expectedLinks[i]
			actualLink := actualLinks[i]

			require.Equal(t, expectedLink.TraceId, actualLink.TraceId)
			require.Equal(t, expectedLink.SpanId, actualLink.SpanId)
			require.Equal(t, expectedLink.TraceState, actualLink.TraceState)
			CompareAttributes(t, expectedLink.Attributes, actualLink.Attributes)
		}
	})
}

func CompareAttributes(t *testing.T, excepted AttributesGroup, actual AttributesGroup) {
	t.Helper()
	assert.ElementsMatch(t, excepted.BoolKeys, actual.BoolKeys)
	assert.ElementsMatch(t, excepted.BoolValues, actual.BoolValues)
	assert.ElementsMatch(t, excepted.DoubleKeys, actual.DoubleKeys)
	assert.ElementsMatch(t, excepted.DoubleValues, actual.DoubleValues)
	assert.ElementsMatch(t, excepted.IntKeys, actual.IntKeys)
	assert.ElementsMatch(t, excepted.IntValues, actual.IntValues)
	assert.ElementsMatch(t, excepted.StrKeys, actual.StrKeys)
	assert.ElementsMatch(t, excepted.StrValues, actual.StrValues)
	assert.ElementsMatch(t, excepted.BytesKeys, actual.BytesKeys)
	assert.ElementsMatch(t, excepted.BytesValues, actual.BytesValues)
}

func Test_encodeTraceID(t *testing.T) {
	type args struct {
		id pcommon.TraceID
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "empty",
			args: args{
				pcommon.NewTraceIDEmpty(),
			},
			want: nil,
		},
		{
			name: "successful",
			args: args{
				pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
			},
			want: []byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, encodeTraceID(tt.args.id), "encodeTraceID(%v)", tt.args.id)
		})
	}
}

func Test_encodeSpanID(t *testing.T) {
	type args struct {
		id pcommon.SpanID
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "empty",
			args: args{
				pcommon.NewSpanIDEmpty(),
			},
			want: nil,
		},
		{
			name: "successful",
			args: args{
				pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}),
			},
			want: []byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, encodeSpanID(tt.args.id), "encodeSpanID(%v)", tt.args.id)
		})
	}
}
