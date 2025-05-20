// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestTODBModel_Fixtures(t *testing.T) {
	trace := jsonToPtrace(t, "./fixtures/ptrace.json")
	actual := ToDBModel(trace)[0]
	expected := jsonToDBModel(t, "./fixtures/dbmodel.json")

	compareResource(t, expected.Resource, actual.Resource)
	compareScope(t, expected.Scope, actual.Scope)
	compareSpan(t, expected.Span, actual.Span)

	require.Len(t, expected.Events, len(actual.Events))
	for i := range expected.Events {
		compareEvent(t, expected.Events[i], actual.Events[i])
	}

	require.Len(t, expected.Links, len(actual.Links))
	for i := range expected.Links {
		compareLink(t, expected.Links[i], actual.Links[i])
	}
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

func compareResource(t *testing.T, expected Resource, actual Resource) {
	t.Helper()
	compareAttributes(t, expected.Attributes, actual.Attributes)
}

func compareScope(t *testing.T, expected Scope, actual Scope) {
	t.Helper()
	require.Equal(t, expected.Name, actual.Name)
	require.Equal(t, expected.Version, actual.Version)
	compareAttributes(t, expected.Attributes, actual.Attributes)
}

func compareSpan(t *testing.T, expected Span, actual Span) {
	t.Helper()
	require.Equal(t, expected.StartTime, actual.StartTime)
	require.Equal(t, expected.TraceId, actual.TraceId)
	require.Equal(t, expected.SpanId, actual.SpanId)
	require.Equal(t, expected.ParentSpanId, actual.ParentSpanId)
	require.Equal(t, expected.Name, actual.Name)
	require.Equal(t, expected.Kind, actual.Kind)
	require.Equal(t, expected.Duration, actual.Duration)
	require.Equal(t, expected.StatusCode, actual.StatusCode)
	require.Equal(t, expected.ServiceName, actual.ServiceName)
	require.Equal(t, expected.StatusMessage, actual.StatusMessage)
	compareAttributes(t, expected.Attributes, actual.Attributes)
}

func compareEvent(t *testing.T, expected Event, actual Event) {
	t.Helper()
	require.Equal(t, expected.Name, actual.Name)
	require.Equal(t, expected.Timestamp, actual.Timestamp)
	compareAttributes(t, expected.Attributes, actual.Attributes)
}

func compareLink(t *testing.T, expected Link, actual Link) {
	t.Helper()
	require.Equal(t, expected.TraceId, actual.TraceId)
	require.Equal(t, expected.SpanId, actual.SpanId)
	require.Equal(t, expected.TraceState, actual.TraceState)
	compareAttributes(t, expected.Attributes, actual.Attributes)
}

func compareAttributes(t *testing.T, excepted AttributesGroup, actual AttributesGroup) {
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
