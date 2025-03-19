// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jpcommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/proto-gen/storage/v2"
)

func TestPcommonMapToPlainMap(t *testing.T) {
	tests := []struct {
		name       string
		attributes pcommon.Map
		expected   map[string]string
	}{
		{
			name:       "empty attributes",
			attributes: pcommon.NewMap(),
			expected:   map[string]string{},
		},
		{
			name: "single attribute",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "value1")
				return m
			}(),
			expected: map[string]string{"key1": "value1"},
		},
		{
			name: "multiple attributes",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "value1")
				m.PutStr("key2", "value2")
				return m
			}(),
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name: "non-string attributes",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutInt("key1", 1)
				m.PutDouble("key2", 3.14)
				return m
			}(),
			expected: map[string]string{"key1": "1", "key2": "3.14"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := PcommonMapToPlainMap(test.attributes)
			require.Equal(t, test.expected, result)
		})
	}
}

func TestPlainMapToPcommonMap(t *testing.T) {
	tests := []struct {
		name     string
		expected map[string]string
	}{
		{
			name:     "empty map",
			expected: map[string]string{},
		},
		{
			name:     "single attribute",
			expected: map[string]string{"key1": "value1"},
		},
		{
			name:     "multiple attributes",
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := PlainMapToPcommonMap(test.expected)
			require.Equal(t, test.expected, PcommonMapToPlainMap(result))
		})
	}
}

func TestConvertMapToKeyValueList(t *testing.T) {
	tests := []struct {
		name       string
		attributes pcommon.Map
		expected   []*storage.KeyValue
	}{
		{
			name:       "empty map",
			attributes: pcommon.NewMap(),
			expected:   []*storage.KeyValue{},
		},
		{
			name: "primitive types",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "value1")
				m.PutInt("key2", 42)
				m.PutDouble("key3", 3.14)
				m.PutBool("key4", true)
				m.PutEmptyBytes("key5").Append(1, 2)
				return m
			}(),
			expected: []*storage.KeyValue{
				{
					Key: "key1",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_StringValue{
							StringValue: "value1",
						},
					},
				},
				{
					Key: "key2",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_IntValue{
							IntValue: 42,
						},
					},
				},
				{
					Key: "key3",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_DoubleValue{
							DoubleValue: 3.14,
						},
					},
				},
				{
					Key: "key4",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_BoolValue{
							BoolValue: true,
						},
					},
				},
				{
					Key: "key5",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_BytesValue{
							BytesValue: []byte{1, 2},
						},
					},
				},
			},
		},
		{
			name: "nested map",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				nested := pcommon.NewMap()
				nested.PutStr("nestedKey", "nestedValue")
				nested.CopyTo(m.PutEmptyMap("key1"))
				return m
			}(),
			expected: []*storage.KeyValue{
				{
					Key: "key1",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_KvlistValue{
							KvlistValue: &storage.KeyValueList{
								Values: []*storage.KeyValue{
									{
										Key: "nestedKey",
										Value: &storage.AnyValue{
											Value: &storage.AnyValue_StringValue{
												StringValue: "nestedValue",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "array attribute",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				arr := pcommon.NewValueSlice()
				arr.Slice().AppendEmpty().SetStr("value1")
				arr.Slice().AppendEmpty().SetInt(42)
				arr.Slice().CopyTo(m.PutEmptySlice("key1"))
				return m
			}(),
			expected: []*storage.KeyValue{
				{
					Key: "key1",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_ArrayValue{
							ArrayValue: &storage.ArrayValue{
								Values: []*storage.AnyValue{
									{
										Value: &storage.AnyValue_StringValue{
											StringValue: "value1",
										},
									},
									{
										Value: &storage.AnyValue_IntValue{
											IntValue: 42,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ConvertMapToKeyValueList(test.attributes)
			require.Equal(t, test.expected, result)
		})
	}
}
