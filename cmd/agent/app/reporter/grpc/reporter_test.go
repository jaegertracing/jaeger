// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	jThrift "github.com/jaegertracing/jaeger-idl/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
)

type mockSpanHandler struct {
	mux      sync.Mutex
	requests []*api_v2.PostSpansRequest
}

func (h *mockSpanHandler) getRequests() []*api_v2.PostSpansRequest {
	h.mux.Lock()
	defer h.mux.Unlock()
	return h.requests
}

func (h *mockSpanHandler) PostSpans(_ context.Context, r *api_v2.PostSpansRequest) (*api_v2.PostSpansResponse, error) {
	h.mux.Lock()
	defer h.mux.Unlock()
	h.requests = append(h.requests, r)
	return &api_v2.PostSpansResponse{}, nil
}

func TestReporter_EmitZipkinBatch(t *testing.T) {
	handler := &mockSpanHandler{}
	s, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
		api_v2.RegisterCollectorServiceServer(s, handler)
	})
	defer s.Stop()
	conn, err := grpc.NewClient(addr.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	rep := NewReporter(conn, nil, zap.NewNop())

	tm := time.Unix(158, 0)
	a := tm.Unix() * 1000 * 1000
	tests := []struct {
		in       *zipkincore.Span
		expected model.Batch
		err      string
	}{
		{in: &zipkincore.Span{}, err: "cannot find service name in Zipkin span [traceID=0, spanID=0]"},
		{
			in: &zipkincore.Span{Name: "jonatan", TraceID: 1, ID: 2, Timestamp: &a, Annotations: []*zipkincore.Annotation{{Value: zipkincore.CLIENT_SEND, Host: &zipkincore.Endpoint{ServiceName: "spring"}}}},
			expected: model.Batch{
				Spans: []*model.Span{{
					TraceID: model.NewTraceID(0, 1), SpanID: model.NewSpanID(2), OperationName: "jonatan", Duration: time.Microsecond * 1,
					Tags:    model.KeyValues{model.SpanKindTag(model.SpanKindClient)},
					Process: &model.Process{ServiceName: "spring"}, StartTime: tm.UTC(),
				}},
			},
		},
	}
	for _, test := range tests {
		err = rep.EmitZipkinBatch(context.Background(), []*zipkincore.Span{test.in})
		if test.err != "" {
			require.EqualError(t, err, test.err)
		} else {
			assert.Len(t, handler.requests, 1)
			assert.Equal(t, test.expected, handler.requests[0].GetBatch())
		}
	}
}

func TestReporter_EmitBatch(t *testing.T) {
	handler := &mockSpanHandler{}
	s, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
		api_v2.RegisterCollectorServiceServer(s, handler)
	})
	defer s.Stop()
	conn, err := grpc.NewClient(addr.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	rep := NewReporter(conn, nil, zap.NewNop())

	tm := time.Unix(158, 0)
	tests := []struct {
		in       *jThrift.Batch
		expected model.Batch
		err      string
	}{
		{
			in:       &jThrift.Batch{Process: &jThrift.Process{ServiceName: "node"}, Spans: []*jThrift.Span{{OperationName: "foo", StartTime: int64(model.TimeAsEpochMicroseconds(tm))}}},
			expected: model.Batch{Process: &model.Process{ServiceName: "node"}, Spans: []*model.Span{{OperationName: "foo", StartTime: tm.UTC()}}},
		},
	}
	for _, test := range tests {
		err = rep.EmitBatch(context.Background(), test.in)
		if test.err != "" {
			require.EqualError(t, err, test.err)
		} else {
			assert.Len(t, handler.requests, 1)
			assert.Equal(t, test.expected, handler.requests[0].GetBatch())
		}
	}
}

func TestReporter_SendFailure(t *testing.T) {
	conn, err := grpc.NewClient("invalid-host-name-blah:12345", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()
	rep := NewReporter(conn, nil, zap.NewNop())
	err = rep.send(context.Background(), nil, nil)
	assert.ErrorContains(t, err, "failed to export spans:")
}

func TestReporter_AddProcessTags_EmptyTags(t *testing.T) {
	tags := map[string]string{}
	spans := []*model.Span{{TraceID: model.NewTraceID(0, 1), SpanID: model.NewSpanID(2), OperationName: "jonatan"}}
	actualSpans, _ := addProcessTags(spans, nil, makeModelKeyValue(tags))
	assert.Equal(t, spans, actualSpans)
}

func TestReporter_AddProcessTags_ZipkinBatch(t *testing.T) {
	tags := map[string]string{"key": "value"}
	spans := []*model.Span{{TraceID: model.NewTraceID(0, 1), SpanID: model.NewSpanID(2), OperationName: "jonatan", Process: &model.Process{ServiceName: "spring"}}}

	expectedSpans := []*model.Span{
		{
			TraceID:       model.NewTraceID(0, 1),
			SpanID:        model.NewSpanID(2),
			OperationName: "jonatan",
			Process:       &model.Process{ServiceName: "spring", Tags: []model.KeyValue{model.String("key", "value")}},
		},
	}
	actualSpans, _ := addProcessTags(spans, nil, makeModelKeyValue(tags))

	assert.Equal(t, expectedSpans, actualSpans)
}

func TestReporter_AddProcessTags_JaegerBatch(t *testing.T) {
	tags := map[string]string{"key": "value"}
	spans := []*model.Span{{TraceID: model.NewTraceID(0, 1), SpanID: model.NewSpanID(2), OperationName: "jonatan"}}
	process := &model.Process{ServiceName: "spring"}

	expectedProcess := &model.Process{ServiceName: "spring", Tags: []model.KeyValue{model.String("key", "value")}}
	_, actualProcess := addProcessTags(spans, process, makeModelKeyValue(tags))

	assert.Equal(t, expectedProcess, actualProcess)
}

func TestReporter_MakeModelKeyValue(t *testing.T) {
	expectedTags := []model.KeyValue{model.String("key", "value")}
	stringTags := map[string]string{"key": "value"}
	actualTags := makeModelKeyValue(stringTags)

	assert.Equal(t, expectedTags, actualTags)
}

type mockMultitenantSpanHandler struct{}

func (*mockMultitenantSpanHandler) PostSpans(ctx context.Context, _ *api_v2.PostSpansRequest) (*api_v2.PostSpansResponse, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return &api_v2.PostSpansResponse{}, status.Errorf(codes.PermissionDenied, "missing tenant header")
	}

	tenants := md["x-tenant"]
	if len(tenants) < 1 {
		return &api_v2.PostSpansResponse{}, status.Errorf(codes.PermissionDenied, "missing tenant header")
	} else if len(tenants) > 1 {
		return &api_v2.PostSpansResponse{}, status.Errorf(codes.PermissionDenied, "extra tenant header")
	}

	return &api_v2.PostSpansResponse{}, nil
}

func TestReporter_MultitenantEmitBatch(t *testing.T) {
	handler := &mockMultitenantSpanHandler{}
	s, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
		api_v2.RegisterCollectorServiceServer(s, handler)
	})
	defer s.Stop()
	conn, err := grpc.NewClient(addr.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	rep := NewReporter(conn, nil, zap.NewNop())

	tm := time.Now()
	tests := []struct {
		in  *jThrift.Batch
		err string
	}{
		{
			in:  &jThrift.Batch{Process: &jThrift.Process{ServiceName: "node"}, Spans: []*jThrift.Span{{OperationName: "foo", StartTime: int64(model.TimeAsEpochMicroseconds(tm))}}},
			err: "missing tenant header",
		},
	}
	for _, test := range tests {
		err = rep.EmitBatch(context.Background(), test.in)
		assert.ErrorContains(t, err, test.err)
	}
}
