// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dbmodel

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	inStr, err := ioutil.ReadFile(in)
	require.NoError(t, err)
	out := fmt.Sprintf("fixtures/es_%02d.json", i)
	outStr, err := ioutil.ReadFile(out)
	require.NoError(t, err)
	return inStr, outStr
}

func testJSONEncoding(t *testing.T, i int, expectedStr []byte, object interface{}) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")

	outFile := fmt.Sprintf("fixtures/es_%02d", i)
	require.NoError(t, enc.Encode(object))

	if !assert.Equal(t, string(expectedStr), buf.String()) {
		err := ioutil.WriteFile(outFile+"-actual.json", buf.Bytes(), 0644)
		assert.NoError(t, err)
	}
}

func TestEmptyTags(t *testing.T) {
	tags := make([]model.KeyValue, 0)
	span := model.Span{Tags: tags, Process: &model.Process{Tags: tags}}
	converter := NewFromDomain(false, nil, ":")
	dbSpan := converter.FromDomainEmbedProcess(&span)
	assert.Equal(t, 0, len(dbSpan.Tags))
	assert.Equal(t, 0, len(dbSpan.Tag))
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

	assert.Equal(t, 1, len(dbSpan.Tags))
	assert.Equal(t, "foo", dbSpan.Tags[0].Key)
	assert.Equal(t, 1, len(dbSpan.Process.Tags))
	assert.Equal(t, "foo", dbSpan.Process.Tags[0].Key)

	tagsMap := map[string]interface{}{}
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
