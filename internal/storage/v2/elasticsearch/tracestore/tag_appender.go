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

type tagAppender struct {
	allTagsAsFields   bool
	tagKeysAsFields   map[string]bool
	tagDotReplacement string
	tagsMap           map[string]any
	tags              []dbmodel.KeyValue
}

func newTagAppender(allTagsAsFields bool, tagKeysAsFields map[string]bool, tagDotReplacement string) *tagAppender {
	return &tagAppender{
		allTagsAsFields:   allTagsAsFields,
		tagKeysAsFields:   tagKeysAsFields,
		tagDotReplacement: tagDotReplacement,
		tagsMap:           make(map[string]any),
		tags:              make([]dbmodel.KeyValue, 0),
	}
}

func (t *tagAppender) getTags() ([]dbmodel.KeyValue, map[string]any) {
	return t.tags, t.tagsMap
}

func (t *tagAppender) appendTags(attrs pcommon.Map) {
	for key, attr := range attrs.All() {
		t.appendTag(key, attr)
	}
}

func (t *tagAppender) appendTag(key string, val pcommon.Value) {
	if val.Type() != pcommon.ValueTypeBytes && (t.allTagsAsFields || t.tagKeysAsFields[key]) {
		t.tagsMap[strings.ReplaceAll(key, ".", t.tagDotReplacement)] = attributeToDbValue(val)
	} else {
		t.tags = append(t.tags, t.attributeToDbTag(key, val.Type(), val.AsString()))
	}
}

func (t *tagAppender) appendSpanKindTag(spanKind ptrace.SpanKind) {
	tagStr := getDbSpanKind(spanKind)
	if tagStr != "" {
		t.appendTag(model.SpanKindKey, pcommon.NewValueStr(tagStr))
	}
}

func (t *tagAppender) appendInstrumentationLibraryTags(il pcommon.InstrumentationScope) {
	if ilName := il.Name(); ilName != "" {
		t.appendTag(conventions.AttributeOtelScopeName, pcommon.NewValueStr(ilName))
	}
	if ilVersion := il.Version(); ilVersion != "" {
		t.appendTag(conventions.AttributeOtelScopeVersion, pcommon.NewValueStr(ilVersion))
	}
}

func (t *tagAppender) appendStatusCodeTag(statusCode ptrace.StatusCode) {
	switch statusCode {
	case ptrace.StatusCodeError:
		t.appendTag(conventions.OtelStatusCode, pcommon.NewValueStr(statusError))
	case ptrace.StatusCodeOk:
		t.appendTag(conventions.OtelStatusCode, pcommon.NewValueStr(statusOk))
	}
}

func (t *tagAppender) appendStatusMsgTag(statusMsg string) {
	if statusMsg != "" {
		t.appendTag(conventions.OtelStatusDescription, pcommon.NewValueStr(statusMsg))
	}
}

func (t *tagAppender) appendTraceStateTag(traceState string) {
	if traceState != "" {
		// TODO Bring this inline with solution for jaegertracing/jaeger-client-java #702 once available
		t.appendTag(tagW3CTraceState, pcommon.NewValueStr(traceState))
	}
}

func getDbSpanKind(spanKind ptrace.SpanKind) string {
	switch spanKind {
	case ptrace.SpanKindClient:
		return string(model.SpanKindClient)
	case ptrace.SpanKindServer:
		return string(model.SpanKindServer)
	case ptrace.SpanKindProducer:
		return string(model.SpanKindProducer)
	case ptrace.SpanKindConsumer:
		return string(model.SpanKindConsumer)
	case ptrace.SpanKindInternal:
		return string(model.SpanKindInternal)
	default:
		return string(model.SpanKindUnspecified)
	}
}

func (*tagAppender) attributeToDbTag(key string, tp pcommon.ValueType, value string) dbmodel.KeyValue {
	// TODO why are all values being converted to strings?
	tag := dbmodel.KeyValue{Key: key, Value: value}
	tag.Type = attributeToDbType(tp)
	return tag
}

func attributeToDbType(tp pcommon.ValueType) dbmodel.ValueType {
	switch tp {
	case pcommon.ValueTypeStr:
		return dbmodel.StringType
	case pcommon.ValueTypeInt:
		return dbmodel.Int64Type
	case pcommon.ValueTypeBool:
		return dbmodel.BoolType
	case pcommon.ValueTypeDouble:
		return dbmodel.Float64Type
	case pcommon.ValueTypeBytes:
		return dbmodel.BinaryType
	case pcommon.ValueTypeMap, pcommon.ValueTypeSlice:
		return dbmodel.StringType
	default:
		return ""
	}
}

func attributeToDbValue(attr pcommon.Value) any {
	switch attr.Type() {
	case pcommon.ValueTypeInt:
		return attr.Int()
	case pcommon.ValueTypeBool:
		return attr.Bool()
	case pcommon.ValueTypeDouble:
		return attr.Double()
	case pcommon.ValueTypeBytes:
		return attr.Bytes()
	case pcommon.ValueTypeStr:
		return attr.Str()
	case pcommon.ValueTypeMap, pcommon.ValueTypeSlice:
		return attr.AsString()
	default:
		return attr.AsString()
	}
}
