// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestConvertToTraces(t *testing.T) {
	t.Run("should return error when traceId is empty", func(t *testing.T) {
		model := Model{TraceId: "00000000000000000000000000000000"}
		_, err := model.ConvertToTraces()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trace id is empty")
	})

	t.Run("should return error when spanId is empty", func(t *testing.T) {
		model := Model{TraceId: "00010001000100010001000100010001", SpanId: "0000000000000000"}
		_, err := model.ConvertToTraces()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "span id is empty")
	})

	t.Run("should return error when parentSpanId is empty", func(t *testing.T) {
		model := Model{TraceId: "00010001000100010001000100010001", SpanId: "0000000000000001", ParentSpanId: "00000000"}
		_, err := model.ConvertToTraces()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parentSpan id is empty")
	})
}

func TestConvertLink(t *testing.T) {
	links := ptrace.NewSpanLink()
	t.Run("should return error when linksTraceId and linksSpanId are empty", func(t *testing.T) {
		model := Model{LinksTraceId: []string{""}, LinksSpanId: []string{""}}
		err := model.convertLink(links, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid trace id or span id")
	})
	t.Run("should return error when linksTraceId is invalid", func(t *testing.T) {
		model := Model{LinksTraceId: []string{"invalidTraceId"}, LinksSpanId: []string{"0x1"}}
		err := model.convertLink(links, 0)
		assert.Error(t, err)
	})
	t.Run("should return error when linksSpanId is invalid", func(t *testing.T) {
		model := Model{LinksTraceId: []string{"0x1"}, LinksSpanId: []string{"invalidSpanId"}}
		err := model.convertLink(links, 0)
		assert.Error(t, err)
	})
}

func TestStatusCode(t *testing.T) {
	t.Run("should return StatusCodeError when status code is 'Error'", func(t *testing.T) {
		model := Model{StatusCode: "Error"}
		assert.Equal(t, ptrace.StatusCodeError, model.statusCode())
	})
	t.Run("should return StatusCodeUnset when status code is 'Unset'", func(t *testing.T) {
		model := Model{StatusCode: "Unset"}
		assert.Equal(t, ptrace.StatusCodeUnset, model.statusCode())
	})
	t.Run("should return StatusCodeOk when status code is 'Ok'", func(t *testing.T) {
		model := Model{StatusCode: "Ok"}
		assert.Equal(t, ptrace.StatusCodeOk, model.statusCode())
	})
	t.Run("should return invalid status code (-1) when status code is unknown", func(t *testing.T) {
		model := Model{StatusCode: "unknown"}
		assert.Equal(t, ptrace.StatusCode(-1), model.statusCode())
	})
}

func TestSpanKind(t *testing.T) {
	t.Run("should return SpanKindUnspecified when span kind is 'UNSPECIFIED'", func(t *testing.T) {
		model := Model{SpanKind: "UNSPECIFIED"}
		assert.Equal(t, ptrace.SpanKindUnspecified, model.spanKind())
	})
	t.Run("should return SpanKindInternal when span kind is 'Internal'", func(t *testing.T) {
		model := Model{SpanKind: "Internal"}
		assert.Equal(t, ptrace.SpanKindInternal, model.spanKind())
	})
	t.Run("should return SpanKindServer when span kind is 'Service'", func(t *testing.T) {
		model := Model{SpanKind: "Service"}
		assert.Equal(t, ptrace.SpanKindServer, model.spanKind())
	})
	t.Run("should return SpanKindClient when span kind is 'Client'", func(t *testing.T) {
		model := Model{SpanKind: "Client"}
		assert.Equal(t, ptrace.SpanKindClient, model.spanKind())
	})
	t.Run("should return SpanKindProducer when span kind is 'Producer'", func(t *testing.T) {
		model := Model{SpanKind: "Producer"}
		assert.Equal(t, ptrace.SpanKindProducer, model.spanKind())
	})
	t.Run("should return SpanKindConsumer when span kind is 'Consumer'", func(t *testing.T) {
		model := Model{SpanKind: "Consumer"}
		assert.Equal(t, ptrace.SpanKindConsumer, model.spanKind())
	})
	t.Run("should return invalid span kind (-1) when span kind is unknown", func(t *testing.T) {
		model := Model{SpanKind: "Unknown"}
		assert.Equal(t, ptrace.SpanKind(-1), model.spanKind())
	})
}
