// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestAttributesToGroup(t *testing.T) {
	type args struct {
		attributes pcommon.Map
	}
	tests := []struct {
		name string
		args args
		want AttributesGroup
	}{
		{
			name: "should successfully when all value are meaningful",
			args: args{
				attributes: func() pcommon.Map {
					m := pcommon.NewMap()

					m.PutEmptySlice("string-slices").FromRaw([]any{"world", "shi jie"})
					m.PutEmptySlice("double-slices").FromRaw([]any{6.824, 0.81})
					m.PutEmptySlice("int-slices").FromRaw([]any{2, 3, 3, 3})
					m.PutEmptySlice("bool-slices").FromRaw([]any{true, false})

					m.PutEmptyMap("string-map").FromRaw(map[string]any{"hello": "world"})
					m.PutEmptyMap("double-map").FromRaw(map[string]any{"sys": 6.824})
					m.PutEmptyMap("int-map").FromRaw(map[string]any{"times": 1})
					m.PutEmptyMap("bool-map").FromRaw(map[string]any{"enable": true})

					m.PutEmptyBytes("string-value-with-bytes").FromRaw([]byte("as you can see."))
					m.PutEmptyBytes("int-value-with-bytes").FromRaw([]byte{1, 2, 3, 4})
					return m
				}(),
			},
			want: AttributesGroup{
				BytesKeys:   []string{"int-value-with-bytes", "string-value-with-bytes"},
				BytesValues: []string{"AQIDBA==", "YXMgeW91IGNhbiBzZWUu"},
				MapKeys:     []string{"string-map", "double-map", "int-map", "bool-map"},
				MapValues:   []string{"{\"hello\":\"world\"}", "{\"sys\":6.824}", "{\"times\":1}", "{\"enable\":true}"},
				SliceKeys:   []string{"int-slices", "double-slices", "string-slices", "bool-slices"},
				SliceValues: []string{"[2,3,3,3]", "[6.824,0.81]", "[\"world\",\"shi jie\"]", "[true,false]"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := AttributesToGroup(tt.args.attributes)
			expect := tt.want

			fields := []struct {
				name     string
				expected []string
				actual   []string
			}{
				{"compare slice keys", expect.SliceKeys, actual.SliceKeys},
				{"compare slice values", expect.SliceValues, actual.SliceValues},
				{"compare map keys", expect.MapKeys, actual.MapKeys},
				{"compare map values", expect.MapValues, actual.MapValues},
				{"compare bytes keys", expect.BytesKeys, actual.BytesKeys},
				{"compare bytes values", expect.BytesValues, actual.BytesValues},
			}

			for _, field := range fields {
				require.ElementsMatch(t, field.expected, field.actual, field.name)
			}
		})
	}
}

func TestAttributesToMap(t *testing.T) {
	type args struct {
		attributes pcommon.Map
	}
	tests := []struct {
		name string
		args args
		want map[pcommon.ValueType]map[string]string
	}{
		{
			name: "should successfully when all values are meaningful",
			args: args{
				attributes: func() pcommon.Map {
					m := pcommon.NewMap()

					m.PutEmptySlice("string-slices").FromRaw([]any{"world", "shi jie"})
					m.PutEmptySlice("double-slices").FromRaw([]any{6.824, 0.81})
					m.PutEmptySlice("int-slices").FromRaw([]any{2, 3, 3, 3})
					m.PutEmptySlice("bool-slices").FromRaw([]any{true, false})

					m.PutEmptyMap("string-map").FromRaw(map[string]any{"hello": "world"})
					m.PutEmptyMap("double-map").FromRaw(map[string]any{"sys": 6.824})
					m.PutEmptyMap("int-map").FromRaw(map[string]any{"times": 1})
					m.PutEmptyMap("bool-map").FromRaw(map[string]any{"enable": true})

					m.PutEmptyBytes("string-value-with-bytes").FromRaw([]byte("as you can see."))
					m.PutEmptyBytes("int-value-with-bytes").FromRaw([]byte{1, 2, 3, 4})
					return m
				}(),
			},
			want: map[pcommon.ValueType]map[string]string{
				pcommon.ValueTypeSlice: {"string-slices": "[\"world\",\"shi jie\"]", "double-slices": "[6.824,0.81]", "int-slices": "[2,3,3,3]", "bool-slices": "[true,false]"},
				pcommon.ValueTypeMap:   {"string-map": "{\"hello\":\"world\"}", "double-map": "{\"sys\":6.824}", "int-map": "{\"times\":1}", "bool-map": "{\"enable\":true}"},
				pcommon.ValueTypeBytes: {"int-value-with-bytes": "AQIDBA==", "string-value-with-bytes": "YXMgeW91IGNhbiBzZWUu"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := AttributesToMap(tt.args.attributes)
			expect := tt.want

			fields := []struct {
				name     string
				expected map[string]string
				actual   map[string]string
			}{
				{name: "compare slice values", expected: expect[pcommon.ValueTypeSlice], actual: actual[pcommon.ValueTypeSlice]},
				{name: "compare map values", expected: expect[pcommon.ValueTypeMap], actual: actual[pcommon.ValueTypeMap]},
				{name: "compare bytes values", expected: expect[pcommon.ValueTypeBytes], actual: actual[pcommon.ValueTypeBytes]},
			}

			for _, field := range fields {
				require.Equal(t, field.expected, field.actual, field.name)
			}
		})
	}
}
