// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/hex"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

// tagAppender append tags to dbmodel KeyValue slice and tagsMap by replacing dots with tagDotReplacement
type tagAppender struct {
	allTagsAsFields   bool
	tagKeysAsFields   map[string]bool
	tagDotReplacement string
	tagsMap           map[string]any
	tags              []dbmodel.KeyValue
}

// newTagAppender return an instance of tagAppender
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
		t.tagsMap[strings.ReplaceAll(key, ".", t.tagDotReplacement)] = val.AsRaw()
	} else {
		t.tags = append(t.tags, attributeToDbTag(key, val))
	}
}

func getStringValue(val pcommon.Value) string {
	if val.Type() == pcommon.ValueTypeBytes {
		return hex.EncodeToString(val.Bytes().AsRaw())
	}
	return val.AsString()
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
		t.appendTag(tagError, pcommon.NewValueBool(true))
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

func attributeToDbTag(key string, value pcommon.Value) dbmodel.KeyValue {
	val := getStringValue(value)
	tp := attributeToDbType(value.Type())
	// TODO why are all values being converted to strings?
	tag := dbmodel.KeyValue{Key: key, Type: tp, Value: val}
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
