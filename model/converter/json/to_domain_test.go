// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package json

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/model"
	jModel "github.com/uber/jaeger/model/json"
)

func TestToDomainEmbeddedProcess(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		span, err := createGoodSpan(i)
		require.NoError(t, err)

		actualSpan, err := SpanToDomain(&span)
		require.NoError(t, err)

		out := fmt.Sprintf("fixtures/domain_es_%02d.json", i)
		outStr, err := ioutil.ReadFile(out)
		require.NoError(t, err)
		var expectedSpan model.Span
		require.NoError(t, json.Unmarshal(outStr, &expectedSpan))

		CompareModelSpans(t, &expectedSpan, actualSpan)
	}
	// this is just to confirm the uint64 representation of float64(72.5) used as a "temperature" tag
	assert.Equal(t, int64(4634802150889750528), model.Float64("x", 72.5).VNum)
}

func createGoodSpan(i int) (jModel.Span, error) {
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
	badTagESSpan, err := createGoodSpan(1)
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
	badTagESSpan, err := createGoodSpan(1)
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
	badTagESSpan, err := createGoodSpan(1)
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
	badTagESSpan, err := createGoodSpan(1)
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
	badTagESSpan, err := createGoodSpan(1)
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
	badLogsESSpan, err := createGoodSpan(1)
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
	assert.EqualError(t, err, "not a valid ValueType string <invalid>")
}

func TestFailureBadRefs(t *testing.T) {
	badRefsESSpan, err := createGoodSpan(1)
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
	badRefsESSpan, err := createGoodSpan(1)
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
	badRefsESSpan, err := createGoodSpan(1)
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
	badProcessESSpan, err := createGoodSpan(1)
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
	badProcessESSpan, err := createGoodSpan(1)
	require.NoError(t, err)

	badProcessESSpan.Process = nil
	failingSpanTransform(t, &badProcessESSpan, "Process is nil")
}

func TestFailureBadTraceID(t *testing.T) {
	badTraceIDESSpan, err := createGoodSpan(1)
	require.NoError(t, err)
	badTraceIDESSpan.TraceID = "zz"
	failingSpanTransformAnyMsg(t, &badTraceIDESSpan)
}

func TestFailureBadSpanID(t *testing.T) {
	badSpanIDESSpan, err := createGoodSpan(1)
	require.NoError(t, err)
	badSpanIDESSpan.SpanID = "zz"
	failingSpanTransformAnyMsg(t, &badSpanIDESSpan)
}

func TestFailureBadParentSpanID(t *testing.T) {
	badParentSpanIDESSpan, err := createGoodSpan(1)
	require.NoError(t, err)
	badParentSpanIDESSpan.ParentSpanID = "zz"
	failingSpanTransformAnyMsg(t, &badParentSpanIDESSpan)
}