// Copyright (c) 2019 The Jaeger Authors.
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

package model_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

// Verify: https://cryptii.com/base64-to-hex
var testCasesTraceID = []struct {
	hi, lo uint64
	hex    string
	b64    string
}{
	{lo: 1, hex: "1", b64: "AAAAAAAAAAAAAAAAAAAAAQ=="},
	{lo: 15, hex: "f", b64: "AAAAAAAAAAAAAAAAAAAADw=="},
	{lo: 31, hex: "1f", b64: "AAAAAAAAAAAAAAAAAAAAHw=="},
	{lo: 257, hex: "101", b64: "AAAAAAAAAAAAAAAAAAABAQ=="},
	{hi: 1, lo: 1, hex: "10000000000000001", b64: "AAAAAAAAAAEAAAAAAAAAAQ=="},
	{hi: 257, lo: 1, hex: "1010000000000000001", b64: "AAAAAAAAAQEAAAAAAAAAAQ=="},
}

func TestTraceIDMarshalJSONPB(t *testing.T) {
	for _, testCase := range testCasesTraceID {
		t.Run(testCase.hex, func(t *testing.T) {
			expected := fmt.Sprintf(`{"traceId":"%s"}`, testCase.b64)

			ref := model.SpanRef{TraceID: model.NewTraceID(testCase.hi, testCase.lo)}
			out := new(bytes.Buffer)
			err := new(jsonpb.Marshaler).Marshal(out, &ref)
			if assert.NoError(t, err) {
				assert.Equal(t, expected, out.String())
				assert.Equal(t, testCase.hex, ref.TraceID.String())
			}

			ref = model.SpanRef{}
			err = jsonpb.Unmarshal(bytes.NewReader([]byte(expected)), &ref)
			if assert.NoError(t, err) {
				assert.Equal(t, testCase.hi, ref.TraceID.High)
				assert.Equal(t, testCase.lo, ref.TraceID.Low)
			}
			traceID, err := model.TraceIDFromString(testCase.hex)
			if assert.NoError(t, err) {
				assert.Equal(t, ref.TraceID, traceID)
			}
		})
	}
}

func TestTraceIDUnmarshalJSONPBErrors(t *testing.T) {
	testCases := []struct {
		in string
	}{
		{in: ""},
		{in: "x"},
		{in: "x0000000000000001"},
		{in: "1x000000000000001"},
		{in: "10123456789abcdef0123456789abcdef"},
		{in: "AAAAAAE="},
	}
	for _, testCase := range testCases {
		t.Run(testCase.in, func(t *testing.T) {
			var ref model.SpanRef
			json := fmt.Sprintf(`{"traceId":"%s"}`, testCase.in)
			err := jsonpb.Unmarshal(bytes.NewReader([]byte(json)), &ref)
			assert.Error(t, err)

			_, err = model.TraceIDFromString(testCase.in)
			assert.Error(t, err)
		})
	}
	// for code coverage
	var id model.TraceID
	_, err := id.MarshalText()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported method")
	err = id.UnmarshalText(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported method")
	_, err = id.MarshalTo(make([]byte, 1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "buffer is too short")
}

var maxSpanID = int64(-1)
var testCasesSpanID = []struct {
	id  uint64
	hex string
	b64 string
}{
	{id: 1, hex: "1", b64: "AAAAAAAAAAE="},
	{id: 15, hex: "f", b64: "AAAAAAAAAA8="},
	{id: 31, hex: "1f", b64: "AAAAAAAAAB8="},
	{id: 257, hex: "101", b64: "AAAAAAAAAQE="},
	{id: uint64(maxSpanID), hex: "ffffffffffffffff", b64: "//////////8="},
}

func TestSpanIDMarshalJSON(t *testing.T) {
	for _, testCase := range testCasesSpanID {
		expected := fmt.Sprintf(`{"traceId":"AAAAAAAAAAAAAAAAAAAAAA==","spanId":"%s"}`, testCase.b64)
		t.Run(testCase.hex, func(t *testing.T) {
			ref := model.SpanRef{SpanID: model.SpanID(testCase.id)}
			out := new(bytes.Buffer)
			err := new(jsonpb.Marshaler).Marshal(out, &ref)
			if assert.NoError(t, err) {
				assert.Equal(t, expected, out.String())
			}
			assert.Equal(t, testCase.hex, ref.SpanID.String())

			ref = model.SpanRef{}
			err = jsonpb.Unmarshal(bytes.NewReader([]byte(expected)), &ref)
			if assert.NoError(t, err) {
				assert.Equal(t, model.NewSpanID(testCase.id), ref.SpanID)
			}
			spanID, err := model.SpanIDFromString(testCase.hex)
			if assert.NoError(t, err) {
				assert.Equal(t, model.NewSpanID(testCase.id), spanID)
			}
		})
	}
}

func TestSpanIDUnmarshalJSONErrors(t *testing.T) {
	testCases := []struct {
		in  string
		err bool
	}{
		{err: true, in: ""},
		{err: true, in: "x"},
		{err: true, in: "x123"},
		{err: true, in: "10123456789abcdef"},
	}
	for _, testCase := range testCases {
		in := fmt.Sprintf(`{"traceId":"0","spanId":"%s"}`, testCase.in)
		t.Run(in, func(t *testing.T) {
			var ref model.SpanRef
			err := jsonpb.Unmarshal(bytes.NewReader([]byte(in)), &ref)
			assert.Error(t, err)

			_, err = model.SpanIDFromString(testCase.in)
			assert.Error(t, err)
		})
	}
	// for code coverage
	var id model.SpanID
	_, err := id.MarshalText()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported method")
	err = id.UnmarshalText(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported method")

	err = id.UnmarshalJSONPB(nil, []byte(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid length for SpanID")
	err = id.UnmarshalJSONPB(nil, []byte("123"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "illegal base64 data")
}

func TestIsRPCClientServer(t *testing.T) {
	span1 := &model.Span{
		Tags: model.KeyValues{
			model.String(string(ext.SpanKind), string(ext.SpanKindRPCClientEnum)),
		},
	}
	assert.True(t, span1.IsRPCClient())
	assert.False(t, span1.IsRPCServer())
	span2 := &model.Span{}
	assert.False(t, span2.IsRPCClient())
	assert.False(t, span2.IsRPCServer())
}

func TestIsDebug(t *testing.T) {
	flags := model.Flags(0)
	flags.SetDebug()
	assert.True(t, flags.IsDebug())
	flags = model.Flags(0)
	assert.False(t, flags.IsDebug())

	flags = model.Flags(32)
	assert.False(t, flags.IsDebug())
	flags.SetDebug()
	assert.True(t, flags.IsDebug())
}

func TestIsFirehoseEnabled(t *testing.T) {
	flags := model.Flags(0)
	assert.False(t, flags.IsFirehoseEnabled())
	flags.SetDebug()
	flags.SetSampled()
	assert.False(t, flags.IsFirehoseEnabled())
	flags.SetFirehose()
	assert.True(t, flags.IsFirehoseEnabled())

	flags = model.Flags(8)
	assert.True(t, flags.IsFirehoseEnabled())
}

func TestGetSpanKind(t *testing.T) {
	span := makeSpan(model.String("sampler.type", "lowerbound"))
	spanKind, found := span.GetSpanKind()
	assert.Equal(t, "", spanKind)
	assert.Equal(t, false, found)

	span = makeSpan(model.String("span.kind", "client"))
	spanKind, found = span.GetSpanKind()
	assert.Equal(t, "client", spanKind)
	assert.Equal(t, true, found)
}

func TestSamplerType(t *testing.T) {
	span := makeSpan(model.String("sampler.type", "lowerbound"))
	assert.Equal(t, "lowerbound", span.GetSamplerType())
	span = makeSpan(model.String("sampler.type", ""))
	assert.Equal(t, "unknown", span.GetSamplerType())
	span = makeSpan(model.KeyValue{})
	assert.Equal(t, "unknown", span.GetSamplerType())
}

func TestIsSampled(t *testing.T) {
	flags := model.Flags(0)
	flags.SetSampled()
	assert.True(t, flags.IsSampled())
	flags = model.Flags(0)
	flags.SetDebug()
	assert.False(t, flags.IsSampled())
}

func TestSpanHash(t *testing.T) {
	kvs := model.KeyValues{
		model.String("x", "y"),
		model.String("x", "y"),
		model.String("x", "z"),
	}
	spans := make([]*model.Span, len(kvs))
	codes := make([]uint64, len(kvs))
	// create 3 spans that are only different in some KeyValues
	for i := range kvs {
		spans[i] = makeSpan(kvs[i])
		hc, err := model.HashCode(spans[i])
		require.NoError(t, err)
		codes[i] = hc
	}
	assert.Equal(t, codes[0], codes[1])
	assert.NotEqual(t, codes[0], codes[2])
}

func TestParentSpanID(t *testing.T) {
	span := makeSpan(model.String("k", "v"))
	assert.Equal(t, model.NewSpanID(123), span.ParentSpanID())

	span.References = []model.SpanRef{
		model.NewFollowsFromRef(span.TraceID, model.NewSpanID(777)),
		model.NewChildOfRef(span.TraceID, model.NewSpanID(888)),
	}
	assert.Equal(t, model.NewSpanID(888), span.ParentSpanID())

	span.References = []model.SpanRef{
		model.NewChildOfRef(model.NewTraceID(321, 0), model.NewSpanID(999)),
	}
	assert.Equal(t, model.NewSpanID(0), span.ParentSpanID())
}

func TestReplaceParentSpanID(t *testing.T) {
	span := makeSpan(model.String("k", "v"))
	assert.Equal(t, model.NewSpanID(123), span.ParentSpanID())

	span.ReplaceParentID(789)
	assert.Equal(t, model.NewSpanID(789), span.ParentSpanID())

	span.References = []model.SpanRef{
		model.NewChildOfRef(model.NewTraceID(321, 0), model.NewSpanID(999)),
	}
	span.ReplaceParentID(789)
	assert.Equal(t, model.NewSpanID(789), span.ParentSpanID())
}

func makeSpan(someKV model.KeyValue) *model.Span {
	traceID := model.NewTraceID(0, 123)
	return &model.Span{
		TraceID:       traceID,
		SpanID:        model.NewSpanID(567),
		OperationName: "hi",
		References:    []model.SpanRef{model.NewChildOfRef(traceID, model.NewSpanID(123))},
		StartTime:     time.Unix(0, 1000),
		Duration:      5000,
		Tags:          model.KeyValues{someKV},
		Logs: []model.Log{
			{
				Timestamp: time.Unix(0, 1000),
				Fields:    model.KeyValues{someKV},
			},
		},
		Process: &model.Process{
			ServiceName: "xyz",
			Tags:        model.KeyValues{someKV},
		},
	}
}

// BenchmarkSpanHash-8   	   50000	     26977 ns/op	    2203 B/op	      68 allocs/op
func BenchmarkSpanHash(b *testing.B) {
	span := makeSpan(model.String("x", "y"))
	buf := &bytes.Buffer{}
	for i := 0; i < b.N; i++ {
		buf.Reset()
		span.Hash(buf)
	}
}
