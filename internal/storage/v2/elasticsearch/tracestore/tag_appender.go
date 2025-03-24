// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

type TagAppender struct {
	allTagsAsFields   bool
	tagKeysAsFields   map[string]bool
	tagDotReplacement string
	tagsMap           map[string]any
	tags              []dbmodel.KeyValue
}

func NewTagAppender(allTagsAsFields bool, tagKeysAsFields map[string]bool, tagDotReplacement string, tagsMap map[string]any, tags []dbmodel.KeyValue) TagAppender {
	if tagsMap == nil {
		tagsMap = make(map[string]any)
	}
	if tags == nil {
		tags = make([]dbmodel.KeyValue, 0)
	}
	return TagAppender{
		allTagsAsFields:   allTagsAsFields,
		tagKeysAsFields:   tagKeysAsFields,
		tagDotReplacement: tagDotReplacement,
		tagsMap:           tagsMap,
		tags:              tags,
	}
}

func (t TagAppender) AppendTagsForSpan(span ptrace.Span, scope pcommon.InstrumentationScope) {
	t.appendTagsFromScope(scope)
	t.appendSpanKind(span.Kind())
	status := span.Status()
	t.appendTagFromStatusCode(status.Code())
	t.appendTagFromStatusMsg(status.Message())
	t.appendTagsFromTraceState(span.TraceState().AsRaw())
	attrs := span.Attributes()
	if attrs.Len() != 0 {
		attrs.Range(func(k string, v pcommon.Value) bool {
			t.appendTagsForDb(k, v.Type(), v.AsString())
			return true
		})
	}
}

func (t TagAppender) AppendTagsFromResourceAttributes(attrs pcommon.Map) {
	attrs.Range(func(key string, attr pcommon.Value) bool {
		if key == conventions.AttributeServiceName {
			return true
		}
		t.appendTagsForDb(key, attr.Type(), attr.AsString())
		return true
	})
}

func (t TagAppender) appendTagsFromScope(scope pcommon.InstrumentationScope) {
	if ilName := scope.Name(); ilName != "" {
		t.appendTagsForDb(conventions.AttributeOtelScopeName, pcommon.ValueTypeStr, ilName)
	}
	if ilVersion := scope.Version(); ilVersion != "" {
		t.appendTagsForDb(conventions.AttributeOtelScopeVersion, pcommon.ValueTypeStr, ilVersion)
	}
}

func (t TagAppender) appendSpanKind(spanKind ptrace.SpanKind) {
	tag, found := toJaegerSpanKind(spanKind)
	if found {
		t.appendTagsForDb(model.SpanKindKey, pcommon.ValueTypeStr, string(tag))
	}
}

func (t TagAppender) appendTagFromStatusCode(statusCode ptrace.StatusCode) {
	key := conventions.OtelStatusCode
	valueType := pcommon.ValueTypeStr
	switch statusCode {
	case ptrace.StatusCodeError:
		t.appendTagsForDb(key, valueType, statusError)
		t.appendTagsForDb(tagError, pcommon.ValueTypeBool, "true")
	case ptrace.StatusCodeOk:
		t.appendTagsForDb(key, valueType, statusOk)
	}
}

func (t TagAppender) appendTagFromStatusMsg(statusMsg string) {
	if statusMsg != "" {
		t.appendTagsForDb(conventions.OtelStatusDescription, pcommon.ValueTypeStr, statusMsg)
	}
}

func (t TagAppender) appendTagsFromTraceState(traceState string) {
	exists := traceState != ""
	if exists {
		t.appendTagsForDb(tagW3CTraceState, pcommon.ValueTypeStr, traceState)
	}
}

func (t TagAppender) appendTagsForDb(key string, valueType pcommon.ValueType, value string) {
	if valueType != pcommon.ValueTypeBytes && (t.allTagsAsFields || t.tagKeysAsFields[key]) {
		t.tagsMap[strings.ReplaceAll(key, ".", t.tagDotReplacement)] = value
	} else {
		t.tags = append(t.tags, dbmodel.KeyValue{
			Key:   key,
			Type:  dbmodel.ValueType(valueType.String()),
			Value: value,
		})
	}
}

func toJaegerSpanKind(spanKind ptrace.SpanKind) (model.SpanKind, bool) {
	switch spanKind {
	case ptrace.SpanKindClient:
		return model.SpanKindClient, true
	case ptrace.SpanKindServer:
		return model.SpanKindServer, true
	case ptrace.SpanKindProducer:
		return model.SpanKindProducer, true
	case ptrace.SpanKindConsumer:
		return model.SpanKindConsumer, true
	case ptrace.SpanKindInternal:
		return model.SpanKindInternal, true
	default:
		return model.SpanKindUnspecified, false
	}
}
