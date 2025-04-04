// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

func TestAppendStatusCodeTag(t *testing.T) {
	tests := []struct {
		name string
		code ptrace.StatusCode
		tag  dbmodel.KeyValue
	}{
		{
			name: "ok",
			code: ptrace.StatusCodeOk,
			tag: dbmodel.KeyValue{
				Key:   conventions.OtelStatusCode,
				Type:  dbmodel.StringType,
				Value: statusOk,
			},
		},

		{
			name: "error",
			code: ptrace.StatusCodeError,
			tag: dbmodel.KeyValue{
				Key:   tagError,
				Type:  dbmodel.BoolType,
				Value: "true",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			appender := newTagAppender(false, nil, "")
			appender.appendStatusCodeTag(test.code)
			got, tagMap := appender.getTags()
			_, ok := tagMap[test.tag.Key]
			assert.False(t, ok)
			assert.Equal(t, test.tag, got[0])
		})
	}
}

func TestAppendStatusMsgTag(t *testing.T) {
	appender := newTagAppender(true, nil, ".")
	appender.appendStatusMsgTag("")
	got, tagMap := appender.getTags()
	assert.Empty(t, tagMap)
	assert.Empty(t, got)
	appender.appendStatusMsgTag("test-error")
	assert.Empty(t, got)
	tag, ok := tagMap[conventions.OtelStatusDescription]
	assert.True(t, ok)
	assert.EqualValues(t, "test-error", tag)
}

func TestAppendSpanKindTag(t *testing.T) {
	tests := []struct {
		name  string
		kind  ptrace.SpanKind
		value string
		ok    bool
	}{
		{
			name:  "unspecified",
			kind:  ptrace.SpanKindUnspecified,
			value: "",
			ok:    false,
		},

		{
			name:  "client",
			kind:  ptrace.SpanKindClient,
			value: string(model.SpanKindClient),
			ok:    true,
		},

		{
			name:  "server",
			kind:  ptrace.SpanKindServer,
			value: string(model.SpanKindServer),
			ok:    true,
		},

		{
			name:  "producer",
			kind:  ptrace.SpanKindProducer,
			value: string(model.SpanKindProducer),
			ok:    true,
		},

		{
			name:  "consumer",
			kind:  ptrace.SpanKindConsumer,
			value: string(model.SpanKindConsumer),
			ok:    true,
		},

		{
			name:  "internal",
			kind:  ptrace.SpanKindInternal,
			value: string(model.SpanKindInternal),
			ok:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			appender := newTagAppender(true, nil, "#")
			appender.appendSpanKindTag(test.kind)
			tags, tagMap := appender.getTags()
			assert.Empty(t, tags)
			tag, ok := tagMap["span#kind"]
			assert.Equal(t, test.ok, ok)
			if test.ok {
				assert.EqualValues(t, test.value, tag)
			}
		})
	}
}

func TestAppendTagWhenTagKeysAsField(t *testing.T) {
	tagKeysAsFields := map[string]bool{
		"testing.key.1": true,
	}
	appender := newTagAppender(false, tagKeysAsFields, "#")
	appender.appendTag("testing.key.1", pcommon.NewValueInt(1))
	appender.appendTag("testing.key.2", pcommon.NewValueInt(2))
	tags, tagMap := appender.getTags()
	assert.Len(t, tags, 1)
	assert.Len(t, tagMap, 1)
	assert.Equal(t, int64(1), tagMap["testing#key#1"])
	expected := dbmodel.KeyValue{Key: "testing.key.2", Type: "int64", Value: "2"}
	assert.Equal(t, expected, tags[0])
}

func TestAppendTag(t *testing.T) {
	t.Run("allTagsAsFields=true", func(t *testing.T) {
		testAppendTags(t, true)
	})
	t.Run("allTagsAsFields=false", func(t *testing.T) {
		testAppendTags(t, false)
	})
}

func testAppendTags(t *testing.T, allTagsAsFields bool) {
	tests := []struct {
		name   string
		value  pcommon.Value
		dbType dbmodel.ValueType
	}{
		{
			name:   "int.val",
			value:  pcommon.NewValueInt(1),
			dbType: dbmodel.Int64Type,
		},
		{
			name:   "string.val",
			value:  pcommon.NewValueStr("testing-string"),
			dbType: dbmodel.StringType,
		},
		{
			name:   "bool.val",
			value:  pcommon.NewValueBool(true),
			dbType: dbmodel.BoolType,
		},
		{
			name:   "double.val",
			value:  pcommon.NewValueDouble(1.2),
			dbType: dbmodel.Float64Type,
		},
		{
			name:   "bytes.val",
			value:  pcommon.NewValueBytes(),
			dbType: dbmodel.BinaryType,
		},
		{
			name:   "map.val",
			value:  pcommon.NewValueMap(),
			dbType: dbmodel.StringType,
		},
		{
			name:   "slice.val",
			value:  pcommon.NewValueSlice(),
			dbType: dbmodel.StringType,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			appender := newTagAppender(allTagsAsFields, nil, ".")
			appender.appendTag(test.name, test.value)
			tags, tagMap := appender.getTags()
			if allTagsAsFields && test.dbType != dbmodel.BinaryType {
				assert.Empty(t, tags)
				assert.Len(t, tagMap, 1)
				switch test.dbType {
				case dbmodel.StringType:
					assert.Equal(t, test.value.AsString(), tagMap[test.name])
				case dbmodel.Int64Type:
					assert.Equal(t, test.value.Int(), tagMap[test.name])
				case dbmodel.BoolType:
					assert.Equal(t, test.value.Bool(), tagMap[test.name])
				case dbmodel.Float64Type:
					assert.InDelta(t, test.value.Double(), tagMap[test.name], 0.01)
				default:
					t.Errorf("unknown db type: %v", test.dbType)
				}
			} else {
				assert.Len(t, tags, 1)
				assert.Empty(t, tagMap)
				expected := dbmodel.KeyValue{
					Key:   test.name,
					Type:  test.dbType,
					Value: test.value.AsString(),
				}
				assert.Equal(t, expected, tags[0])
			}
		})
	}
}

func TestAttributeToDbType_UnknownType(t *testing.T) {
	dbType := attributeToDbType(pcommon.ValueType(13))
	assert.Equal(t, dbmodel.ValueType(""), dbType)
}

func TestAttributeToDbValue_UnknownAndBinaryType(t *testing.T) {
	val := pcommon.NewValueEmpty()
	binaryVal := pcommon.NewValueBytes()
	dbValue := attributeToDbValue(val)
	dbBinaryVal := attributeToDbValue(binaryVal)
	assert.Equal(t, val.AsString(), dbValue)
	assert.Equal(t, binaryVal.Bytes(), dbBinaryVal)
}
