// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
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
	{lo: 1, hex: "0000000000000001", b64: "AAAAAAAAAAAAAAAAAAAAAQ=="},
	{lo: 15, hex: "000000000000000f", b64: "AAAAAAAAAAAAAAAAAAAADw=="},
	{lo: 31, hex: "000000000000001f", b64: "AAAAAAAAAAAAAAAAAAAAHw=="},
	{lo: 257, hex: "0000000000000101", b64: "AAAAAAAAAAAAAAAAAAABAQ=="},
	{hi: 1, lo: 1, hex: "00000000000000010000000000000001", b64: "AAAAAAAAAAEAAAAAAAAAAQ=="},
	{hi: 257, lo: 1, hex: "00000000000001010000000000000001", b64: "AAAAAAAAAQEAAAAAAAAAAQ=="},
}

func TestTraceIDMarshalJSONPB(t *testing.T) {
	for _, testCase := range testCasesTraceID {
		t.Run(testCase.hex, func(t *testing.T) {
			expected := fmt.Sprintf(`{"traceId":"%s"}`, testCase.b64)

			ref := model.SpanRef{TraceID: model.NewTraceID(testCase.hi, testCase.lo)}
			out := new(bytes.Buffer)
			err := new(jsonpb.Marshaler).Marshal(out, &ref)
			require.NoError(t, err)
			assert.Equal(t, expected, out.String())
			assert.Equal(t, testCase.hex, ref.TraceID.String())

			ref = model.SpanRef{}
			err = jsonpb.Unmarshal(bytes.NewReader([]byte(expected)), &ref)
			require.NoError(t, err)
			assert.Equal(t, testCase.hi, ref.TraceID.High)
			assert.Equal(t, testCase.lo, ref.TraceID.Low)
			traceID, err := model.TraceIDFromString(testCase.hex)
			require.NoError(t, err)
			assert.Equal(t, ref.TraceID, traceID)
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
			require.Error(t, err)

			_, err = model.TraceIDFromString(testCase.in)
			require.Error(t, err)
		})
	}
	// for code coverage
	var id model.TraceID
	_, err := id.MarshalText()
	require.ErrorContains(t, err, "unsupported method")
	err = id.UnmarshalText(nil)
	require.ErrorContains(t, err, "unsupported method")
	_, err = id.MarshalTo(make([]byte, 1))
	assert.ErrorContains(t, err, "buffer is too short")
}

var (
	maxSpanID       = int64(-1)
	testCasesSpanID = []struct {
		id  uint64
		hex string
		b64 string
	}{
		{id: 1, hex: "0000000000000001", b64: "AAAAAAAAAAE="},
		{id: 15, hex: "000000000000000f", b64: "AAAAAAAAAA8="},
		{id: 31, hex: "000000000000001f", b64: "AAAAAAAAAB8="},
		{id: 257, hex: "0000000000000101", b64: "AAAAAAAAAQE="},
		{id: uint64(maxSpanID), hex: "ffffffffffffffff", b64: "//////////8="},
	}
)

func TestSpanIDMarshalJSON(t *testing.T) {
	for _, testCase := range testCasesSpanID {
		expected := fmt.Sprintf(`{"traceId":"AAAAAAAAAAAAAAAAAAAAAA==","spanId":"%s"}`, testCase.b64)
		t.Run(testCase.hex, func(t *testing.T) {
			ref := model.SpanRef{SpanID: model.SpanID(testCase.id)}
			out := new(bytes.Buffer)
			err := new(jsonpb.Marshaler).Marshal(out, &ref)
			require.NoError(t, err)
			assert.Equal(t, expected, out.String())
			assert.Equal(t, testCase.hex, ref.SpanID.String())

			ref = model.SpanRef{}
			err = jsonpb.Unmarshal(bytes.NewReader([]byte(expected)), &ref)
			require.NoError(t, err)
			assert.Equal(t, model.NewSpanID(testCase.id), ref.SpanID)
			spanID, err := model.SpanIDFromString(testCase.hex)
			require.NoError(t, err)
			assert.Equal(t, model.NewSpanID(testCase.id), spanID)
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
			require.Error(t, err)

			_, err = model.SpanIDFromString(testCase.in)
			require.Error(t, err)
		})
	}
	// for code coverage
	var id model.SpanID
	_, err := id.MarshalText()
	require.ErrorContains(t, err, "unsupported method")
	err = id.UnmarshalText(nil)
	require.ErrorContains(t, err, "unsupported method")

	err = id.UnmarshalJSONPB(nil, []byte(""))
	require.ErrorContains(t, err, "invalid length for SpanID")
	err = id.UnmarshalJSONPB(nil, []byte("123"))
	assert.ErrorContains(t, err, "illegal base64 data")
}

func TestIsRPCClientServer(t *testing.T) {
	span1 := &model.Span{
		Tags: model.KeyValues{
			model.String(model.SpanKindKey, "client"),
		},
	}
	assert.True(t, span1.IsRPCClient())
	assert.False(t, span1.IsRPCServer())
	span2 := &model.Span{}
	assert.False(t, span2.IsRPCClient())
	assert.False(t, span2.IsRPCServer())
}

func TestGetSpanKind(t *testing.T) {
	span := makeSpan(model.String("sampler.type", "lowerbound"))
	spanKind, found := span.GetSpanKind()
	assert.Equal(t, model.SpanKindUnspecified, spanKind)
	assert.False(t, found)

	span = makeSpan(model.SpanKindTag("client"))
	spanKind, found = span.GetSpanKind()
	assert.Equal(t, model.SpanKindClient, spanKind)
	assert.True(t, found)
}

func TestSamplerType(t *testing.T) {
	span := makeSpan(model.String("sampler.type", "lowerbound"))
	assert.Equal(t, model.SamplerTypeLowerBound, span.GetSamplerType())
	span = makeSpan(model.String("sampler.type", ""))
	assert.Equal(t, model.SamplerTypeUnrecognized, span.GetSamplerType())
	span = makeSpan(model.String("sampler.type", "probabilistic"))
	assert.Equal(t, model.SamplerTypeProbabilistic, span.GetSamplerType())
	span = makeSpan(model.KeyValue{})
	assert.Equal(t, model.SamplerTypeUnrecognized, span.GetSamplerType())
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
	}
	assert.Equal(t, model.NewSpanID(777), span.ParentSpanID())

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

func BenchmarkBatchSerialization(b *testing.B) {
	batch := &model.Batch{
		Spans: []*model.Span{
			{
				TraceID:       model.NewTraceID(154, 1879),
				SpanID:        model.NewSpanID(66974),
				OperationName: "test_op",
				References: []model.SpanRef{
					{
						TraceID: model.NewTraceID(45, 12),
						SpanID:  model.NewSpanID(789),
						RefType: model.SpanRefType_CHILD_OF,
					},
				},
				Flags:     0,
				StartTime: time.Now(),
				Duration:  time.Second,
				Tags: []model.KeyValue{
					model.String("foo", "bar"), model.Bool("haha", true),
				},
				Logs: []model.Log{
					{
						Timestamp: time.Now(),
						Fields: []model.KeyValue{
							model.String("foo", "bar"), model.Int64("bar", 156),
						},
					},
				},
				Process:   model.NewProcess("process1", []model.KeyValue{model.String("aaa", "bbb")}),
				ProcessID: "156",
			},
		},
		Process: nil,
	}

	b.Run("marshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			proto.Marshal(batch)
		}
	})

	data, err := proto.Marshal(batch)
	require.NoError(b, err)
	b.Run("unmarshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var batch2 model.Batch
			proto.Unmarshal(data, &batch2)
		}
	})
}
