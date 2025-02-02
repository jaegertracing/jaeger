// Copyright (c) 2019 The Jaeger Authors
// SPDX-License-Identifier: Apache-2.0

package storage_v1_test

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/grpc/proto/storageprototest"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
)

func TestGetTraceRequestMarshalProto(t *testing.T) {
	testCases := []struct {
		name      string
		marshal   func(proto.Message) ([]byte, error)
		unmarshal func([]byte, proto.Message) error
	}{
		{
			name:      "protobuf",
			marshal:   proto.Marshal,
			unmarshal: proto.Unmarshal,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ref1 := storage_v1.GetTraceRequest{
				TraceID:   model.NewTraceID(2, 3),
				StartTime: time.Unix(1, 2).UTC(),
				EndTime:   time.Unix(3, 4).UTC(),
			}
			ref2 := storageprototest.GetTraceRequest{
				TraceId:   []byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3},
				StartTime: timestamppb.New(time.Unix(1, 2).UTC()),
				EndTime:   timestamppb.New(time.Unix(3, 4).UTC()),
			}
			d1, err := testCase.marshal(&ref1)
			require.NoError(t, err)
			d2, err := testCase.marshal(&ref2)
			require.NoError(t, err)
			assert.Equal(t, d2, d1)

			// test unmarshal
			var ref1u storage_v1.GetTraceRequest
			err = testCase.unmarshal(d2, &ref1u)
			require.NoError(t, err)
			assert.Equal(t, ref1, ref1u)
		})
	}
}

func TestSpansResponseChunkMarshalProto(t *testing.T) {
	testCases := []struct {
		name      string
		marshal   func(proto.Message) ([]byte, error)
		unmarshal func([]byte, proto.Message) error
	}{
		{
			name:      "protobuf",
			marshal:   proto.Marshal,
			unmarshal: proto.Unmarshal,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			span1 := makeSpan(model.KeyValue{Key: "key", VStr: "value"})
			span2 := makeSpan(model.KeyValue{Key: "key2", VStr: "value2"})
			// If we don't normalize timestamps then the tests will fail due to locale not being preserved
			span1.NormalizeTimestamps()
			span2.NormalizeTimestamps()
			ref1 := storage_v1.SpansResponseChunk{Spans: []model.Span{
				span1, span2,
			}}
			d1, err := testCase.marshal(&ref1)
			require.NoError(t, err)

			// test unmarshal
			var ref1u storage_v1.SpansResponseChunk
			err = testCase.unmarshal(d1, &ref1u)
			require.NoError(t, err)
			assert.Equal(t, ref1, ref1u)
		})
	}
}

func makeSpan(someKV model.KeyValue) model.Span {
	traceID := model.NewTraceID(0, 123)
	return model.Span{
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

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
