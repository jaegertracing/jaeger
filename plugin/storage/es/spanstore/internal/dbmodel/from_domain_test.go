// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

const NumberOfFixtures = 1

func TestFromDomainEmbedProcess(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		t.Run(fmt.Sprintf("fixture_%d", i), func(t *testing.T) {
			domainStr, jsonStr := loadFixtures(t, i)

			var span model.Span
			require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(domainStr), &span))
			converter := NewFromDomain(false, nil, ":")
			embeddedSpan := converter.FromDomainEmbedProcess(&span)

			var expectedSpan Span
			require.NoError(t, json.Unmarshal(jsonStr, &expectedSpan))

			testJSONEncoding(t, i, jsonStr, embeddedSpan)

			CompareJSONSpans(t, &expectedSpan, embeddedSpan)
		})
	}
}

// Loads and returns domain model and JSON model fixtures with given number i.
func loadFixtures(t *testing.T, i int) ([]byte, []byte) {
	in := fmt.Sprintf("fixtures/domain_%02d.json", i)
	inStr, err := os.ReadFile(in)
	require.NoError(t, err)
	out := fmt.Sprintf("fixtures/es_%02d.json", i)
	outStr, err := os.ReadFile(out)
	require.NoError(t, err)
	return inStr, outStr
}

func testJSONEncoding(t *testing.T, i int, expectedStr []byte, object any) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")

	outFile := fmt.Sprintf("fixtures/es_%02d", i)
	require.NoError(t, enc.Encode(object))

	if !assert.Equal(t, string(expectedStr), buf.String()) {
		err := os.WriteFile(outFile+"-actual.json", buf.Bytes(), 0o644)
		require.NoError(t, err)
	}
}

func TestEmptyTags(t *testing.T) {
	tags := make([]model.KeyValue, 0)
	span := model.Span{Tags: tags, Process: &model.Process{Tags: tags}}
	converter := NewFromDomain(false, nil, ":")
	dbSpan := converter.FromDomainEmbedProcess(&span)
	assert.Empty(t, dbSpan.Tags)
	assert.Empty(t, dbSpan.Tag)
}

func TestTagMap(t *testing.T) {
	tags := []model.KeyValue{
		model.String("foo", "foo"),
		model.Bool("a", true),
		model.Int64("b.b", 1),
	}
	span := model.Span{Tags: tags, Process: &model.Process{Tags: tags}}
	converter := NewFromDomain(false, []string{"a", "b.b", "b*"}, ":")
	dbSpan := converter.FromDomainEmbedProcess(&span)

	assert.Len(t, dbSpan.Tags, 1)
	assert.Equal(t, "foo", dbSpan.Tags[0].Key)
	assert.Len(t, dbSpan.Process.Tags, 1)
	assert.Equal(t, "foo", dbSpan.Process.Tags[0].Key)

	tagsMap := map[string]any{}
	tagsMap["a"] = true
	tagsMap["b:b"] = int64(1)
	assert.Equal(t, tagsMap, dbSpan.Tag)
	assert.Equal(t, tagsMap, dbSpan.Process.Tag)
}

func TestConvertKeyValueValue(t *testing.T) {
	longString := `Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues
	Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues
	Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues
	Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues
	Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues Bender Bending Rodrigues `
	key := "key"
	tests := []struct {
		kv       model.KeyValue
		expected KeyValue
	}{
		{
			kv:       model.Bool(key, true),
			expected: KeyValue{Key: key, Value: "true", Type: "bool"},
		},
		{
			kv:       model.Bool(key, false),
			expected: KeyValue{Key: key, Value: "false", Type: "bool"},
		},
		{
			kv:       model.Int64(key, int64(1499)),
			expected: KeyValue{Key: key, Value: "1499", Type: "int64"},
		},
		{
			kv:       model.Float64(key, float64(15.66)),
			expected: KeyValue{Key: key, Value: "15.66", Type: "float64"},
		},
		{
			kv:       model.String(key, longString),
			expected: KeyValue{Key: key, Value: longString, Type: "string"},
		},
		{
			kv:       model.Binary(key, []byte(longString)),
			expected: KeyValue{Key: key, Value: hex.EncodeToString([]byte(longString)), Type: "binary"},
		},
		{
			kv:       model.KeyValue{VType: 1500, Key: key},
			expected: KeyValue{Key: key, Value: "unknown type 1500", Type: "1500"},
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s:%s", test.expected.Type, test.expected.Key), func(t *testing.T) {
			actual := convertKeyValue(test.kv)
			assert.Equal(t, test.expected, actual)
		})
	}
}
