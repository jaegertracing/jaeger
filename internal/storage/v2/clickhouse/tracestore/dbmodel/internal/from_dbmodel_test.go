// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestFromDBModel_Fixtures(t *testing.T) {
	dbTrace := jsonToDBModel(t, "./fixtures/dbmodel.json")
	expected := jsonToPtrace(t, "./fixtures/ptrace.json")
	actual := FromDBModel(dbTrace)

	require.Equal(t, expected.ResourceSpans().Len(), actual.ResourceSpans().Len())
	if actual.ResourceSpans().Len() == 0 {
		t.Fatal("Actual trace contains no ResourceSpans")
	}
	expectedResourceSpans := expected.ResourceSpans().At(0)
	actualResourceSpans := actual.ResourceSpans().At(0)
	comparePCommonResource(t, expectedResourceSpans.Resource(), actualResourceSpans.Resource())

	require.Equal(t, expectedResourceSpans.ScopeSpans().Len(), actualResourceSpans.ScopeSpans().Len())
	if actualResourceSpans.ScopeSpans().Len() == 0 {
		t.Fatal("Actual ResourceSpans contains no ScopeSpans")
	}
	exceptedScopeSpans := expectedResourceSpans.ScopeSpans().At(0)
	actualScopeSpans := actualResourceSpans.ScopeSpans().At(0)
	comparePCommonScope(t, exceptedScopeSpans.Scope(), actualScopeSpans.Scope())

	require.Equal(t, exceptedScopeSpans.Spans().Len(), actualScopeSpans.Spans().Len())
	if actualScopeSpans.Spans().Len() == 0 {
		t.Fatal("Actual ScopeSpans contains no Spans")
	}
	exceptedSpan := exceptedScopeSpans.Spans().At(0)
	actualSpan := actualScopeSpans.Spans().At(0)
	comparePTraceSpan(t, exceptedSpan, actualSpan)

	exceptedEvents := exceptedSpan.Events()
	actualEvents := actualSpan.Events()
	require.Equal(t, exceptedEvents.Len(), actualEvents.Len())
	for i := 0; i < exceptedEvents.Len(); i++ {
		exceptedEvent := exceptedEvents.At(i)
		actualEvent := actualEvents.At(i)
		compareSpanEvent(t, exceptedEvent, actualEvent)
	}

	exceptedLinks := exceptedSpan.Links()
	actualLinks := actualSpan.Links()
	require.Equal(t, exceptedLinks.Len(), actualLinks.Len())
	for i := 0; i < exceptedLinks.Len(); i++ {
		exceptedLink := exceptedLinks.At(i)
		actualLink := actualLinks.At(i)
		compareSpanLink(t, exceptedLink, actualLink)
	}
}

func TestFromDBSpanKind(t *testing.T) {
	type args struct {
		sk string
	}
	tests := []struct {
		name string
		args args
		want ptrace.SpanKind
	}{
		{
			name: "Unspecified",
			args: args{
				sk: "Unspecified",
			},
			want: ptrace.SpanKindUnspecified,
		},
		{
			name: "Internal",
			args: args{
				sk: "Internal",
			},
			want: ptrace.SpanKindInternal,
		},
		{
			name: "Server",
			args: args{
				sk: "Server",
			},
			want: ptrace.SpanKindServer,
		},
		{
			name: "Client",
			args: args{
				sk: "Client",
			},
			want: ptrace.SpanKindClient,
		},
		{
			name: "Producer",
			args: args{
				sk: "Producer",
			},
			want: ptrace.SpanKindProducer,
		},
		{
			name: "Consumer",
			args: args{
				sk: "Consumer",
			},
			want: ptrace.SpanKindConsumer,
		},
		{
			name: "Unknown",
			args: args{
				sk: "Unknown",
			},
			want: ptrace.SpanKindUnspecified,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, fromDBSpanKind(tt.args.sk))
		})
	}
}

func TestFromDBStatusCode(t *testing.T) {
	type args struct {
		sc string
	}
	tests := []struct {
		name string
		args args
		want ptrace.StatusCode
	}{
		{
			name: "OK status code",
			args: args{
				sc: "OK",
			},
			want: ptrace.StatusCodeOk,
		},
		{
			name: "Unset status code",
			args: args{
				sc: "Unset",
			},
			want: ptrace.StatusCodeUnset,
		},
		{
			name: "Error status code",
			args: args{
				sc: "Error",
			},
			want: ptrace.StatusCodeError,
		},
		{
			name: "Unknown status code",
			args: args{
				sc: "Unknown",
			},
			want: ptrace.StatusCodeUnset,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, fromDBStatusCode(tt.args.sc))
		})
	}
}

func comparePCommonResource(t *testing.T, expected pcommon.Resource, resource pcommon.Resource) {
	require.Equal(t, expected.Attributes().AsRaw(), resource.Attributes().AsRaw())
}

func comparePCommonScope(t *testing.T, excepted pcommon.InstrumentationScope, actual pcommon.InstrumentationScope) {
	require.Equal(t, excepted.Name(), actual.Name())
	require.Equal(t, excepted.Version(), actual.Version())
	require.Equal(t, excepted.Attributes().AsRaw(), actual.Attributes().AsRaw())
}

func comparePTraceSpan(t *testing.T, excepted ptrace.Span, actual ptrace.Span) {
	require.Equal(t, excepted.StartTimestamp(), actual.StartTimestamp())
	require.Equal(t, excepted.TraceID(), actual.TraceID())
	require.Equal(t, excepted.SpanID(), actual.SpanID())
	require.Equal(t, excepted.ParentSpanID(), actual.ParentSpanID())
	require.Equal(t, excepted.TraceState(), actual.TraceState())
	require.Equal(t, excepted.Name(), actual.Name())
	require.Equal(t, excepted.Kind(), actual.Kind())
	require.Equal(t, excepted.EndTimestamp(), actual.EndTimestamp())
	require.Equal(t, excepted.Status().Code(), actual.Status().Code())
	require.Equal(t, excepted.Status().Message(), actual.Status().Message())
	require.Equal(t, excepted.Attributes().AsRaw(), actual.Attributes().AsRaw())
}

func compareSpanEvent(t *testing.T, excepted ptrace.SpanEvent, actual ptrace.SpanEvent) {
	require.Equal(t, excepted.Name(), actual.Name())
	require.Equal(t, excepted.Timestamp(), actual.Timestamp())
	require.Equal(t, excepted.Attributes().AsRaw(), actual.Attributes().AsRaw())
}

func compareSpanLink(t *testing.T, excepted ptrace.SpanLink, actual ptrace.SpanLink) {
	require.Equal(t, excepted.SpanID(), actual.SpanID())
	require.Equal(t, excepted.TraceID(), actual.TraceID())
	require.Equal(t, excepted.TraceState(), actual.TraceState())
	require.Equal(t, excepted.Attributes().AsRaw(), actual.Attributes().AsRaw())
}
