// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package assembly

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func TestConvertTagField(t *testing.T) {
	dotReplacer := dbmodel.NewDotReplacer("!")

	tests := []struct {
		name          string
		key           string
		value         any
		expectedType  dbmodel.ValueType
		expectedKey   string
		expectedValue any
	}{
		{
			name:          "string value",
			key:           "tag!key",
			value:         "test-value",
			expectedType:  dbmodel.StringType,
			expectedKey:   "tag.key",
			expectedValue: "test-value",
		},
		{
			name:          "bool value true",
			key:           "is!enabled",
			value:         true,
			expectedType:  dbmodel.BoolType,
			expectedKey:   "is.enabled",
			expectedValue: true,
		},
		{
			name:          "bool value false",
			key:           "is!disabled",
			value:         false,
			expectedType:  dbmodel.BoolType,
			expectedKey:   "is.disabled",
			expectedValue: false,
		},
		{
			name:          "int64 value",
			key:           "count!total",
			value:         int64(42),
			expectedType:  dbmodel.Int64Type,
			expectedKey:   "count.total",
			expectedValue: int64(42),
		},
		{
			name:          "float64 value",
			key:           "rate!value",
			value:         3.14,
			expectedType:  dbmodel.Float64Type,
			expectedKey:   "rate.value",
			expectedValue: 3.14,
		},
		{
			name:          "json.Number as int64",
			key:           "num!int",
			value:         json.Number("123"),
			expectedType:  dbmodel.Int64Type,
			expectedKey:   "num.int",
			expectedValue: int64(123),
		},
		{
			name:          "json.Number as float64",
			key:           "num!float",
			value:         json.Number("123.45"),
			expectedType:  dbmodel.Float64Type,
			expectedKey:   "num.float",
			expectedValue: 123.45,
		},
		{
			name:         "json.Number invalid",
			key:          "num!invalid",
			value:        json.Number("not-a-number"),
			expectedType: dbmodel.StringType,
			expectedKey:  "num.invalid",
		},
		{
			name:          "binary value",
			key:           "data!binary",
			value:         []byte{0x01, 0x02, 0x03},
			expectedType:  dbmodel.BinaryType,
			expectedKey:   "data.binary",
			expectedValue: []byte{0x01, 0x02, 0x03},
		},
		{
			name:         "unknown type",
			key:          "unknown!type",
			value:        struct{ Field string }{Field: "test"},
			expectedType: dbmodel.StringType,
			expectedKey:  "unknown.type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertTagField(tt.key, tt.value, dotReplacer)
			assert.Equal(t, tt.expectedKey, result.Key)
			assert.Equal(t, tt.expectedType, result.Type)
			if tt.expectedValue != nil {
				assert.Equal(t, tt.expectedValue, result.Value)
			}
		})
	}
}

func TestMergeNestedAndElevatedTags(t *testing.T) {
	dotReplacer := dbmodel.NewDotReplacer("!")

	tests := []struct {
		name         string
		nestedTags   []dbmodel.KeyValue
		elevatedTags map[string]any
		expectedLen  int
	}{
		{
			name: "empty elevated tags",
			nestedTags: []dbmodel.KeyValue{
				{Key: "nested1", Value: "value1", Type: dbmodel.StringType},
				{Key: "nested2", Value: "value2", Type: dbmodel.StringType},
			},
			elevatedTags: map[string]any{},
			expectedLen:  2,
		},
		{
			name: "non-empty elevated tags",
			nestedTags: []dbmodel.KeyValue{
				{Key: "nested1", Value: "value1", Type: dbmodel.StringType},
			},
			elevatedTags: map[string]any{
				"elevated.key1": "elevated-value1",
				"elevated.key2": int64(42),
			},
			expectedLen: 3,
		},
		{
			name:       "only elevated tags",
			nestedTags: []dbmodel.KeyValue{},
			elevatedTags: map[string]any{
				"elevated.key1": "elevated-value1",
			},
			expectedLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeNestedAndElevatedTags(tt.nestedTags, tt.elevatedTags, dotReplacer)
			assert.Len(t, result, tt.expectedLen)
			// Verify elevated tags map is cleared (intentional side effect)
			assert.Empty(t, tt.elevatedTags)
		})
	}
}

func TestMergeAllNestedAndElevatedTagsOfSpan(t *testing.T) {
	dotReplacer := dbmodel.NewDotReplacer("!")

	tests := []struct {
		name                string
		span                *dbmodel.Span
		expectedSpanTagsLen int
		expectedProcTagsLen int
		elevatedTagsCleared bool
	}{
		{
			name: "span with no elevated tags",
			span: &dbmodel.Span{
				Tags: []dbmodel.KeyValue{
					{Key: "tag1", Value: "value1", Type: dbmodel.StringType},
				},
				Tag: map[string]any{},
				Process: dbmodel.Process{
					Tags: []dbmodel.KeyValue{
						{Key: "proc1", Value: "value1", Type: dbmodel.StringType},
					},
					Tag: map[string]any{},
				},
			},
			expectedSpanTagsLen: 1,
			expectedProcTagsLen: 1,
			elevatedTagsCleared: true,
		},
		{
			name: "span with elevated tags",
			span: &dbmodel.Span{
				Tags: []dbmodel.KeyValue{
					{Key: "tag1", Value: "value1", Type: dbmodel.StringType},
				},
				Tag: map[string]any{
					"elevated.tag": "elevated-value",
				},
				Process: dbmodel.Process{
					Tags: []dbmodel.KeyValue{
						{Key: "proc1", Value: "value1", Type: dbmodel.StringType},
					},
					Tag: map[string]any{
						"elevated.proc": int64(123),
					},
				},
			},
			expectedSpanTagsLen: 2,
			expectedProcTagsLen: 2,
			elevatedTagsCleared: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			MergeAllNestedAndElevatedTagsOfSpan(tt.span, dotReplacer)
			assert.Len(t, tt.span.Tags, tt.expectedSpanTagsLen)
			assert.Len(t, tt.span.Process.Tags, tt.expectedProcTagsLen)
			if tt.elevatedTagsCleared {
				assert.Empty(t, tt.span.Tag)
				assert.Empty(t, tt.span.Process.Tag)
			}
		})
	}
}

func TestSplitElevatedTags(t *testing.T) {
	tests := []struct {
		name              string
		keyValues         []dbmodel.KeyValue
		allTagsAsFields   bool
		tagKeysAsFields   map[string]bool
		tagDotReplacement string
		expectedNested    int
		expectedElevated  int
	}{
		{
			name: "allTagsAsFields true",
			keyValues: []dbmodel.KeyValue{
				{Key: "tag1", Value: "value1", Type: dbmodel.StringType},
				{Key: "tag2", Value: int64(42), Type: dbmodel.Int64Type},
			},
			allTagsAsFields:   true,
			tagKeysAsFields:   map[string]bool{},
			tagDotReplacement: "!",
			expectedNested:    0,
			expectedElevated:  2,
		},
		{
			name: "allTagsAsFields false, key in tagKeysAsFields",
			keyValues: []dbmodel.KeyValue{
				{Key: "tag1", Value: "value1", Type: dbmodel.StringType},
				{Key: "tag2", Value: "value2", Type: dbmodel.StringType},
			},
			allTagsAsFields:   false,
			tagKeysAsFields:   map[string]bool{"tag1": true},
			tagDotReplacement: "!",
			expectedNested:    1,
			expectedElevated:  1,
		},
		{
			name: "allTagsAsFields false, key not in tagKeysAsFields",
			keyValues: []dbmodel.KeyValue{
				{Key: "tag1", Value: "value1", Type: dbmodel.StringType},
				{Key: "tag2", Value: "value2", Type: dbmodel.StringType},
			},
			allTagsAsFields:   false,
			tagKeysAsFields:   map[string]bool{},
			tagDotReplacement: "!",
			expectedNested:    2,
			expectedElevated:  0,
		},
		{
			name:              "empty input",
			keyValues:         []dbmodel.KeyValue{},
			allTagsAsFields:   true,
			tagKeysAsFields:   map[string]bool{},
			tagDotReplacement: "!",
			expectedNested:    0,
			expectedElevated:  0,
		},
		{
			name: "binary type not elevated",
			keyValues: []dbmodel.KeyValue{
				{Key: "binary", Value: []byte{0x01}, Type: dbmodel.BinaryType},
				{Key: "string", Value: "value", Type: dbmodel.StringType},
			},
			allTagsAsFields:   true,
			tagKeysAsFields:   map[string]bool{},
			tagDotReplacement: "!",
			expectedNested:    1,
			expectedElevated:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nested, elevated := SplitElevatedTags(tt.keyValues, tt.allTagsAsFields, tt.tagKeysAsFields, tt.tagDotReplacement)
			assert.Len(t, nested, tt.expectedNested)
			if tt.expectedElevated > 0 {
				require.NotNil(t, elevated)
				assert.Len(t, elevated, tt.expectedElevated)
			} else if elevated != nil {
				assert.Empty(t, elevated)
			}
		})
	}
}

func TestConvertNestedTagsToFieldTags(t *testing.T) {
	tests := []struct {
		name              string
		span              *dbmodel.Span
		allTagsAsFields   bool
		tagKeysAsFields   map[string]bool
		tagDotReplacement string
		expectedSpanTag   int
		expectedProcTag   int
	}{
		{
			name: "span with tags to elevate",
			span: &dbmodel.Span{
				Tags: []dbmodel.KeyValue{
					{Key: "tag1", Value: "value1", Type: dbmodel.StringType},
					{Key: "tag2", Value: "value2", Type: dbmodel.StringType},
				},
				Process: dbmodel.Process{
					Tags: []dbmodel.KeyValue{
						{Key: "proc1", Value: "value1", Type: dbmodel.StringType},
					},
				},
			},
			allTagsAsFields:   true,
			tagKeysAsFields:   map[string]bool{},
			tagDotReplacement: "!",
			expectedSpanTag:   2,
			expectedProcTag:   1,
		},
		{
			name: "span with no tags",
			span: &dbmodel.Span{
				Tags: []dbmodel.KeyValue{},
				Process: dbmodel.Process{
					Tags: []dbmodel.KeyValue{},
				},
			},
			allTagsAsFields:   false,
			tagKeysAsFields:   map[string]bool{},
			tagDotReplacement: "!",
			expectedSpanTag:   0,
			expectedProcTag:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ConvertNestedTagsToFieldTags(tt.span, tt.allTagsAsFields, tt.tagKeysAsFields, tt.tagDotReplacement)
			if tt.expectedSpanTag > 0 {
				require.NotNil(t, tt.span.Tag)
				assert.Len(t, tt.span.Tag, tt.expectedSpanTag)
			}
			if tt.expectedProcTag > 0 {
				require.NotNil(t, tt.span.Process.Tag)
				assert.Len(t, tt.span.Process.Tag, tt.expectedProcTag)
			}
		})
	}
}

func TestLogErrorToSpan(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "non-nil error",
			err:  assert.AnError,
		},
		{
			name: "nil error",
			err:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use noop tracer to create a span
			tracer := noop.NewTracerProvider().Tracer("test")
			_, span := tracer.Start(t.Context(), "test-span")

			// Should not panic for both nil and non-nil errors
			require.NotPanics(t, func() {
				LogErrorToSpan(span, tt.err)
			})
		})
	}
}

func TestKeyInCache(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		setup    func(c cache.Cache)
		expected bool
	}{
		{
			name: "key exists in cache",
			key:  "existing-key",
			setup: func(c cache.Cache) {
				c.Put("existing-key", "value")
			},
			expected: true,
		},
		{
			name:     "key does not exist",
			key:      "non-existing-key",
			setup:    func(_ cache.Cache) {},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cache.NewLRUWithOptions(10, nil)
			tt.setup(c)
			result := KeyInCache(tt.key, c)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWriteCache(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "write a key",
			key:  "test-key",
		},
		{
			name: "write same key twice",
			key:  "duplicate-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cache.NewLRUWithOptions(10, nil)

			// Should not panic
			require.NotPanics(t, func() {
				WriteCache(tt.key, c)
			})

			// Verify key exists in cache
			assert.True(t, KeyInCache(tt.key, c))

			// Write again should not panic
			if tt.name == "write same key twice" {
				require.NotPanics(t, func() {
					WriteCache(tt.key, c)
				})
			}
		})
	}
}
