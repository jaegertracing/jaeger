// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"testing"

	gogojsonpb "github.com/gogo/protobuf/jsonpb"
	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
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
		outStr, err := os.ReadFile(out)
		require.NoError(t, err)
		var expectedSpan model.Span
		require.NoError(t, gogojsonpb.Unmarshal(bytes.NewReader(outStr), &expectedSpan))

		CompareModelSpans(t, &expectedSpan, actualSpan)
	}
}

func loadESSpanFixture(i int) (Span, error) {
	in := fmt.Sprintf("fixtures/es_%02d.json", i)
	inStr, err := os.ReadFile(in)
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
	require.EqualError(t, err, errMsg)
}

func failingSpanTransformAnyMsg(t *testing.T, embeddedSpan *Span) {
	domainSpan, err := NewToDomain(":").SpanToDomain(embeddedSpan)
	assert.Nil(t, domainSpan)
	require.Error(t, err)
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
			assert.ErrorContains(t, err, test.err)
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
	badParentSpanIDESSpan.Tag = map[string]any{"foo": struct{}{}}
	failingSpanTransformAnyMsg(t, &badParentSpanIDESSpan)
}

func TestFailureBadProcessFieldTag(t *testing.T) {
	badParentSpanIDESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badParentSpanIDESSpan.Process.Tag = map[string]any{"foo": struct{}{}}
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
		require.NoError(t, err)
		t.Logf("Actual trace: %s", string(out))
	}
}

func TestTagsMap(t *testing.T) {
	tests := []struct {
		fieldTags map[string]any
		expected  []model.KeyValue
		err       error
	}{
		{fieldTags: map[string]any{"bool:bool": true}, expected: []model.KeyValue{model.Bool("bool.bool", true)}},
		{fieldTags: map[string]any{"int.int": int64(1)}, expected: []model.KeyValue{model.Int64("int.int", 1)}},
		{fieldTags: map[string]any{"int:int": int64(2)}, expected: []model.KeyValue{model.Int64("int.int", 2)}},
		{fieldTags: map[string]any{"float": float64(1.1)}, expected: []model.KeyValue{model.Float64("float", 1.1)}},
		{fieldTags: map[string]any{"float": float64(123)}, expected: []model.KeyValue{model.Float64("float", float64(123))}},
		{fieldTags: map[string]any{"float": float64(123.0)}, expected: []model.KeyValue{model.Float64("float", float64(123.0))}},
		{fieldTags: map[string]any{"float:float": float64(123)}, expected: []model.KeyValue{model.Float64("float.float", float64(123))}},
		{fieldTags: map[string]any{"json_number:int": json.Number("123")}, expected: []model.KeyValue{model.Int64("json_number.int", 123)}},
		{fieldTags: map[string]any{"json_number:float": json.Number("123.0")}, expected: []model.KeyValue{model.Float64("json_number.float", float64(123.0))}},
		{fieldTags: map[string]any{"json_number:err": json.Number("foo")}, err: errors.New("invalid tag type in foo: strconv.ParseFloat: parsing \"foo\": invalid syntax")},
		{fieldTags: map[string]any{"str": "foo"}, expected: []model.KeyValue{model.String("str", "foo")}},
		{fieldTags: map[string]any{"str:str": "foo"}, expected: []model.KeyValue{model.String("str.str", "foo")}},
		{fieldTags: map[string]any{"binary": []byte("foo")}, expected: []model.KeyValue{model.Binary("binary", []byte("foo"))}},
		{fieldTags: map[string]any{"binary:binary": []byte("foo")}, expected: []model.KeyValue{model.Binary("binary.binary", []byte("foo"))}},
		{fieldTags: map[string]any{"unsupported": struct{}{}}, err: fmt.Errorf("invalid tag type in %+v", struct{}{})},
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
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expected, tags)
			}
		})
	}
}

func TestDotReplacement(t *testing.T) {
	converter := NewDotReplacer("#")
	k := "foo.foo"
	assert.Equal(t, k, converter.ReplaceDotReplacement(converter.ReplaceDot(k)))
}
