// Copyright (c) 2017 Uber Technologies, Inc.
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

package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"testing"

	gogojsonpb "github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	jModel "github.com/jaegertracing/jaeger/model/json"
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

		actualSpan, err := SpanToDomain(&span)
		require.NoError(t, err)

		out := fmt.Sprintf("fixtures/domain_es_%02d.json", i)
		outStr, err := ioutil.ReadFile(out)
		require.NoError(t, err)
		var expectedSpan model.Span
		require.NoError(t, gogojsonpb.Unmarshal(bytes.NewReader(outStr), &expectedSpan))

		CompareModelSpans(t, &expectedSpan, actualSpan)
	}
}

func loadESSpanFixture(i int) (jModel.Span, error) {
	in := fmt.Sprintf("fixtures/es_%02d.json", i)
	inStr, err := ioutil.ReadFile(in)
	if err != nil {
		return jModel.Span{}, err
	}
	var span jModel.Span
	err = json.Unmarshal(inStr, &span)
	return span, err
}

func failingSpanTransform(t *testing.T, embeddedSpan *jModel.Span, errMsg string) {
	domainSpan, err := SpanToDomain(embeddedSpan)
	assert.Nil(t, domainSpan)
	assert.EqualError(t, err, errMsg)
}

func failingSpanTransformAnyMsg(t *testing.T, embeddedSpan *jModel.Span) {
	domainSpan, err := SpanToDomain(embeddedSpan)
	assert.Nil(t, domainSpan)
	assert.Error(t, err)
}

func TestFailureBadTypeTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []jModel.KeyValue{
		{
			Key:  "meh",
			Type: "badType",
		},
	}
	failingSpanTransformAnyMsg(t, &badTagESSpan)
}

func TestFailureBadBoolTags(t *testing.T) {
	badTagESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badTagESSpan.Tags = []jModel.KeyValue{
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

	badTagESSpan.Tags = []jModel.KeyValue{
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

	badTagESSpan.Tags = []jModel.KeyValue{
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

	badTagESSpan.Tags = []jModel.KeyValue{
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
	badLogsESSpan.Logs = []jModel.Log{
		{
			Timestamp: 0,
			Fields: []jModel.KeyValue{
				{
					Key:  "sneh",
					Type: "badType",
				},
			},
		},
	}
	failingSpanTransform(t, &badLogsESSpan, "not a valid ValueType string badType")
}

func TestRevertKeyValueOfType(t *testing.T) {
	td := toDomain{}
	tag := &jModel.KeyValue{
		Key:   "sneh",
		Type:  "badType",
		Value: "someString",
	}
	_, err := td.convertKeyValueOfType(tag, model.ValueType(7))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid ValueType string")
}

func TestFailureBadRefs(t *testing.T) {
	badRefsESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)
	badRefsESSpan.References = []jModel.Reference{
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
	badRefsESSpan.References = []jModel.Reference{
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
	badRefsESSpan.References = []jModel.Reference{
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

	badTags := []jModel.KeyValue{
		{
			Key:  "meh",
			Type: "badType",
		},
	}
	badProcessESSpan.Process = &jModel.Process{
		ServiceName: "hello",
		Tags:        badTags,
	}
	failingSpanTransform(t, &badProcessESSpan, "not a valid ValueType string badType")
}

func TestProcessPointer(t *testing.T) {
	badProcessESSpan, err := loadESSpanFixture(1)
	require.NoError(t, err)

	badProcessESSpan.Process = nil
	failingSpanTransform(t, &badProcessESSpan, "Process is nil")
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
