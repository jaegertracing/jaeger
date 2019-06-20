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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"testing"

	gogojsonpb "github.com/gogo/protobuf/jsonpb"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

func TestToDomain(t *testing.T) {
	testToDomain(t, false)
	testToDomain(t, true)
	// this is just to confirm the uint64 representation of float64(72.5) used as a "temperature" tag
	assert.Equal(t, int64(4634802150889750528), int64(math.Float64bits(72.5)))
}

func testToDomain(t *testing.T, testParentSpanID bool) {
	for i := 1; i <= NumberOfFixtures; i++ {
		span, err := loadESSpanFixture(i)
		require.NoError(t, err)
		if testParentSpanID {
			span.ParentSpanID = "3"
		}

		actualSpan, err := NewToDomain(":").SpanToDomain(&span)
		require.NoError(t, err)

		out := fmt.Sprintf("fixtures/domain_%02d.json", i)
		outStr, err := ioutil.ReadFile(out)
		require.NoError(t, err)
		var expectedSpan model.Span
		require.NoError(t, gogojsonpb.Unmarshal(bytes.NewReader(outStr), &expectedSpan))

		CompareModelSpans(t, &expectedSpan, actualSpan)
	}
}

func loadESSpanFixture(i int) (Span, error) {
	in := fmt.Sprintf("fixtures/es_%02d.json", i)
	inStr, err := ioutil.ReadFile(in)
	if err != nil {
		return Span{}, err
	}
	var span Span
	err = json.Unmarshal(inStr, &span)
	return span, err
}

func failingSpanTransform(t *testing.T, embeddedSpan *Span, errMsg string) {
	domainSpan, err := NewToDomain(":").SpanToDomain(embeddedSpan)
	assert.Nil(t, domainSpan)
	assert.EqualError(t, err, errMsg)
}

func failingSpanTransformAnyMsg(t *testing.T, embeddedSpan *Span) {
	domainSpan, err := NewToDomain(":").SpanToDomain(embeddedSpan)
	assert.Nil(t, domainSpan)
	assert.Error(t, err)
}

func TestFailureBadTypeTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []KeyValue{
		{
			Key:   "meh",
			Type:  "badType",
			Value: "",
		},
	}
	failingSpanTransformAnyMsg(t, &badTagESSpan)
}

func TestFailureBadBoolTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []KeyValue{
		{
			Key:   "meh",
			Value: "meh",
			Type:  "bool",
		},
	}
	failingSpanTransformAnyMsg(t, &badTagESSpan)
}

func TestFailureBadIntTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []KeyValue{
		{
			Key:   "meh",
			Value: "meh",
			Type:  "int64",
		},
	}
	failingSpanTransformAnyMsg(t, &badTagESSpan)
}

func TestFailureBadFloatTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []KeyValue{
		{
			Key:   "meh",
			Value: "meh",
			Type:  "float64",
		},
	}
	failingSpanTransformAnyMsg(t, &badTagESSpan)
}

func TestFailureBadBinaryTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []KeyValue{
		{
			Key:   "zzzz",
			Value: "zzzz",
			Type:  "binary",
		},
	}
	failingSpanTransformAnyMsg(t, &badTagESSpan)
}

func TestFailureBadLogs(t *testing.T) {
	badLogsESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badLogsESSpan.Logs = []Log{
		{
			Timestamp: 0,
			Fields: []KeyValue{
				{
					Key:   "sneh",
					Value: "",
					Type:  "badType",
				},
			},
		},
	}
	failingSpanTransform(t, &badLogsESSpan, "not a valid ValueType string badType")
}

func TestRevertKeyValueOfType(t *testing.T) {
	tests := []struct {
		kv  *KeyValue
		err string
	}{
		{
			kv: &KeyValue{
				Key:   "sneh",
				Type:  "badType",
				Value: "someString",
			},
			err: "not a valid ValueType string",
		},
		{
			kv:  &KeyValue{},
			err: "invalid nil Value",
		},
		{
			kv: &KeyValue{
				Value: 123,
			},
			err: "non-string Value of type",
		},
	}
	td := ToDomain{}
	for _, test := range tests {
		t.Run(test.err, func(t *testing.T) {
			tag := test.kv
			_, err := td.convertKeyValue(tag)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.err)
		})
	}
}

func TestFailureBadRefs(t *testing.T) {
	badRefsESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badRefsESSpan.References = []Reference{
		{
			RefType: "makeOurOwnCasino",
			TraceID: "1",
		},
	}
	failingSpanTransform(t, &badRefsESSpan, "not a valid SpanRefType string makeOurOwnCasino")
}

func TestFailureBadTraceIDRefs(t *testing.T) {
	badRefsESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badRefsESSpan.References = []Reference{
		{
			RefType: "CHILD_OF",
			TraceID: "ZZBADZZ",
			SpanID:  "1",
		},
	}
	failingSpanTransformAnyMsg(t, &badRefsESSpan)
}

func TestFailureBadSpanIDRefs(t *testing.T) {
	badRefsESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badRefsESSpan.References = []Reference{
		{
			RefType: "CHILD_OF",
			TraceID: "1",
			SpanID:  "ZZBADZZ",
		},
	}
	failingSpanTransformAnyMsg(t, &badRefsESSpan)
}

func TestFailureBadProcess(t *testing.T) {
	badProcessESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTags := []KeyValue{
		{
			Key:   "meh",
			Value: "",
			Type:  "badType",
		},
	}
	badProcessESSpan.Process = Process{
		ServiceName: "hello",
		Tags:        badTags,
	}
	failingSpanTransform(t, &badProcessESSpan, "not a valid ValueType string badType")
}

func TestFailureBadTraceID(t *testing.T) {
	badTraceIDESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badTraceIDESSpan.TraceID = "zz"
	failingSpanTransformAnyMsg(t, &badTraceIDESSpan)
}

func TestFailureBadSpanID(t *testing.T) {
	badSpanIDESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badSpanIDESSpan.SpanID = "zz"
	failingSpanTransformAnyMsg(t, &badSpanIDESSpan)
}

func TestFailureBadParentSpanID(t *testing.T) {
	badParentSpanIDESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badParentSpanIDESSpan.ParentSpanID = "zz"
	failingSpanTransformAnyMsg(t, &badParentSpanIDESSpan)
}

func TestFailureBadSpanFieldTag(t *testing.T) {
	badParentSpanIDESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badParentSpanIDESSpan.Tag = map[string]interface{}{"foo": struct{}{}}
	failingSpanTransformAnyMsg(t, &badParentSpanIDESSpan)
}

func TestFailureBadProcessFieldTag(t *testing.T) {
	badParentSpanIDESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badParentSpanIDESSpan.Process.Tag = map[string]interface{}{"foo": struct{}{}}
	failingSpanTransformAnyMsg(t, &badParentSpanIDESSpan)
}

func CompareModelSpans(t *testing.T, expected *model.Span, actual *model.Span) {
	model.SortSpan(expected)
	model.SortSpan(actual)

	if !assert.EqualValues(t, expected, actual) {
		for _, err := range pretty.Diff(expected, actual) {
			t.Log(err)
		}
		out, err := json.Marshal(actual)
		assert.NoError(t, err)
		t.Logf("Actual trace: %s", string(out))
	}
}

func TestTagsMap(t *testing.T) {
	tests := []struct {
		fieldTags map[string]interface{}
		expected  []model.KeyValue
		err       error
	}{
		{fieldTags: map[string]interface{}{"bool:bool": true}, expected: []model.KeyValue{model.Bool("bool.bool", true)}},
		{fieldTags: map[string]interface{}{"int.int": int64(1)}, expected: []model.KeyValue{model.Int64("int.int", 1)}},
		{fieldTags: map[string]interface{}{"int:int": int64(2)}, expected: []model.KeyValue{model.Int64("int.int", 2)}},
		{fieldTags: map[string]interface{}{"float": float64(1.1)}, expected: []model.KeyValue{model.Float64("float", 1.1)}},
		{fieldTags: map[string]interface{}{"float": float64(123)}, expected: []model.KeyValue{model.Float64("float", float64(123))}},
		{fieldTags: map[string]interface{}{"float": float64(123.0)}, expected: []model.KeyValue{model.Float64("float", float64(123.0))}},
		{fieldTags: map[string]interface{}{"float:float": float64(123)}, expected: []model.KeyValue{model.Float64("float.float", float64(123))}},
		{fieldTags: map[string]interface{}{"json_number:int": json.Number("123")}, expected: []model.KeyValue{model.Int64("json_number.int", 123)}},
		{fieldTags: map[string]interface{}{"json_number:float": json.Number("123.0")}, expected: []model.KeyValue{model.Float64("json_number.float", float64(123.0))}},
		{fieldTags: map[string]interface{}{"json_number:err": json.Number("foo")}, err: fmt.Errorf("strconv.ParseFloat: parsing \"foo\": invalid syntax")},
		{fieldTags: map[string]interface{}{"str": "foo"}, expected: []model.KeyValue{model.String("str", "foo")}},
		{fieldTags: map[string]interface{}{"str:str": "foo"}, expected: []model.KeyValue{model.String("str.str", "foo")}},
		{fieldTags: map[string]interface{}{"binary": []byte("foo")}, expected: []model.KeyValue{model.Binary("binary", []byte("foo"))}},
		{fieldTags: map[string]interface{}{"binary:binary": []byte("foo")}, expected: []model.KeyValue{model.Binary("binary.binary", []byte("foo"))}},
		{fieldTags: map[string]interface{}{"unsupported": struct{}{}}, err: fmt.Errorf("invalid tag type in %+v", struct{}{})},
	}
	converter := NewToDomain(":")
	for i, test := range tests {
		t.Run(fmt.Sprintf("%d, %s", i, test.fieldTags), func(t *testing.T) {
			tags, err := converter.convertTagFields(test.fieldTags)
			if err != nil {
				fmt.Println(err.Error())
			}
			if test.err != nil {
				assert.Equal(t, test.err.Error(), err.Error())
				require.Nil(t, tags)
			} else  {
				require.NoError(t, err)
				assert.Equal(t, test.expected, tags)
			}
		})
	}
}

func TestDotReplacement(t *testing.T) {
	converter := NewToDomain("#")
	k := "foo.foo"
	assert.Equal(t, k, converter.ReplaceDotReplacement(converter.ReplaceDot(k)))
}
