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

package zipkin

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/model"
	z "github.com/uber/jaeger/thrift-gen/zipkincore"
)

const NumberOfFixtures = 3

func TestToDomain(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		in := fmt.Sprintf("fixtures/zipkin_%02d.json", i)
		out := fmt.Sprintf("fixtures/jaeger_%02d.json", i)
		zSpans := loadZipkinSpans(t, in)
		expectedTrace := loadJaegerTrace(t, out)
		name := in + " -> " + out + " : " + zSpans[0].Name
		t.Run(name, func(t *testing.T) {
			trace, err := ToDomain(zSpans)
			assert.NoError(t, err)
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
			t.Run("ToDomainSpan", func(t *testing.T) {
				zSpan := zSpans[0]
				jSpan, err := ToDomainSpan(zSpan)
				assert.NoError(t, err)
				assert.Equal(t, expectedTrace.Spans[0], jSpan)
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
