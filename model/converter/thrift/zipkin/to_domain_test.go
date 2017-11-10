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

package zipkin

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	z "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

const NumberOfFixtures = 3

func TestToDomain(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		in := fmt.Sprintf("fixtures/zipkin_%02d.json", i)
		out := fmt.Sprintf("fixtures/jaeger_%02d.json", i)
		zSpans := loadZipkinSpans(t, in)
		expectedTrace := loadJaegerTrace(t, out)
		expectedTrace.NormalizeTimestamps()
		name := in + " -> " + out + " : " + zSpans[0].Name
		t.Run(name, func(t *testing.T) {
			trace, err := ToDomain(zSpans)
			assert.NoError(t, err)
			trace.NormalizeTimestamps()
			if !assert.Equal(t, expectedTrace, trace) {
				for _, err := range pretty.Diff(expectedTrace, trace) {
					t.Log(err)
				}
				out, err := json.Marshal(trace)
				assert.NoError(t, err)
				t.Logf("Actual trace: %s", string(out))
			}
		})
		if i == 1 {
			t.Run("ToDomainSpans", func(t *testing.T) {
				zSpan := zSpans[0]
				jSpans, err := ToDomainSpan(zSpan)
				assert.NoError(t, err)
				for _, jSpan := range jSpans {
					jSpan.NormalizeTimestamps()
					assert.Equal(t, expectedTrace.Spans[0], jSpan)
				}
			})
		}
	}
}

func TestToDomainNoServiceNameError(t *testing.T) {
	zSpans := getZipkinSpans(t, `[{ "trace_id": -1, "id": 31 }]`)
	trace, err := ToDomain(zSpans)
	assert.EqualError(t, err, "Cannot find service name in Zipkin span [traceID=ffffffffffffffff, spanID=1f]")
	assert.Equal(t, 1, len(trace.Spans))
	assert.Equal(t, "unknown-service-name", trace.Spans[0].Process.ServiceName)
}

func TestToDomainServiceNameInBinAnnotation(t *testing.T) {
	zSpans := getZipkinSpans(t, `[{ "trace_id": -1, "id": 31,
	"binary_annotations": [{"key": "foo", "host": {"service_name": "bar", "ipv4": 23456}}] }]`)
	trace, err := ToDomain(zSpans)
	require.Nil(t, err)
	assert.Equal(t, 1, len(trace.Spans))
	assert.Equal(t, "bar", trace.Spans[0].Process.ServiceName)
}

func TestToDomainMultipleSpanKinds(t *testing.T) {
	tests := []struct {
		json      string
		tagFirst  opentracing.Tag
		tagSecond opentracing.Tag
	}{
		{json: `[{ "trace_id": -1, "id": 31, "annotations": [
		{"value": "cs", "host": {"service_name": "bar", "ipv4": 23456}},
		{"value": "sr", "timestamp": 1, "host": {"service_name": "bar", "ipv4": 23456}},
		{"value": "ss", "timestamp": 2, "host": {"service_name": "bar", "ipv4": 23456}}
		]}]`,
			tagFirst:  ext.SpanKindRPCClient,
			tagSecond: ext.SpanKindRPCServer,
		},
		{json: `[{ "trace_id": -1, "id": 31, "annotations": [
		{"value": "sr", "host": {"service_name": "bar", "ipv4": 23456}},
		{"value": "cs", "timestamp": 1, "host": {"service_name": "bar", "ipv4": 23456}},
		{"value": "cr", "timestamp": 2, "host": {"service_name": "bar", "ipv4": 23456}}
		]}]`,
			tagFirst:  ext.SpanKindRPCServer,
			tagSecond: ext.SpanKindRPCClient,
		},
	}

	for _, test := range tests {
		fmt.Println(test.json)
		trace, err := ToDomain(getZipkinSpans(t, test.json))
		require.Nil(t, err)

		assert.Equal(t, 2, len(trace.Spans))
		assert.Equal(t, 1, trace.Spans[0].Tags.Len())
		assert.Equal(t, test.tagFirst.Key, trace.Spans[0].Tags[0].Key)
		assert.Equal(t, string(test.tagFirst.Value.(ext.SpanKindEnum)), trace.Spans[0].Tags[0].VStr)

		assert.Equal(t, 1, trace.Spans[1].Tags.Len())
		assert.Equal(t, test.tagSecond.Key, trace.Spans[1].Tags[0].Key)
		assert.Equal(t, time.Duration(1000), trace.Spans[1].Duration)
		assert.Equal(t, string(test.tagSecond.Value.(ext.SpanKindEnum)), trace.Spans[1].Tags[0].VStr)
	}
}

func TestInvalidAnnotationTypeError(t *testing.T) {
	_, err := toDomain{}.transformBinaryAnnotation(&z.BinaryAnnotation{
		AnnotationType: -1,
	})
	assert.EqualError(t, err, "Unknown zipkin annotation type: -1")
}

// TestZipkinEncoding is just for reference to explain the base64 strings
// used in zipkin_03.json and jaeger_03.json fixtures
func TestValidateBase64Values(t *testing.T) {
	numberToBase64 := func(num interface{}) string {
		buf := &bytes.Buffer{}
		binary.Write(buf, binary.BigEndian, num)
		encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
		return encoded
	}

	assert.Equal(t, "MDk=", numberToBase64(int16(12345)))
	assert.Equal(t, "AAAwOQ==", numberToBase64(int32(12345)))
	assert.Equal(t, "AAAAAAAAMDk=", numberToBase64(int64(12345)))
	assert.Equal(t, "QMgcgAAAAAA=", numberToBase64(model.Float64("x", 12345).VNum))
	assert.Equal(t, int64(4668012349850910720), model.Float64("x", 12345).VNum)
}

func loadZipkinSpans(t *testing.T, file string) []*z.Span {
	var zSpans []*z.Span
	loadJSON(t, file, &zSpans)
	return zSpans
}

func loadJaegerTrace(t *testing.T, file string) *model.Trace {
	var trace model.Trace
	loadJSON(t, file, &trace)
	return &trace
}

func getZipkinSpans(t *testing.T, s string) []*z.Span {
	var zSpans []*z.Span
	require.NoError(t, json.Unmarshal([]byte(s), &zSpans))
	return zSpans
}

func loadJSON(t *testing.T, fileName string, i interface{}) {
	jsonFile, err := os.Open(fileName)
	require.NoError(t, err, "Failed to load json fixture file %s", fileName)
	jsonParser := json.NewDecoder(jsonFile)
	err = jsonParser.Decode(i)
	require.NoError(t, err, "Failed to parse json fixture file %s", fileName)
}
