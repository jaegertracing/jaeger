// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"
	"iter"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/jiter"
	storage "github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

func spanOrder() []tracestore.SpanSortOrder {
	return []tracestore.SpanSortOrder{
		{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: "duration"}, Direction: tracestore.SortDescending},
		{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: "traceID"}, Direction: tracestore.SortAscending},
	}
}

func spanSequence(chunks []tracestore.PageChunk[ptrace.Traces], err error) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
		for _, chunk := range chunks {
			if !yield(chunk, nil) {
				return
			}
		}
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
		}
	}
}

func spanRemote(t *testing.T, reader tracestore.Reader) *TraceReader {
	t.Helper()
	server := grpc.NewServer()
	handler := NewHandler(reader, nil, nil)
	storage.RegisterTraceReaderServer(server, handler)
	storage.RegisterCapabilitiesServer(server, handler)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return NewTraceReader(startServer(t, server, listener))
}

func TestSpanRemoteRoundTrip(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "ordered"}[ordered], func(t *testing.T) {
			reader := new(tracestoremocks.Reader)
			caps := tracestore.SearchCapabilities{SpanSearch: true, SpanSorting: true, Paginated: true}
			reader.On("SearchCapabilities", mock.Anything).Return(caps, nil)
			query := tracestore.SpanQueryParams{Pagination: tracestore.Pagination{PageSize: 2, PageToken: "cursor"}}
			if ordered {
				query.OrderBy = spanOrder()
			}
			chunks := []tracestore.PageChunk[ptrace.Traces]{{Results: makeTestTrace()}, {Results: ptrace.NewTraces(), NextPageToken: "next"}}
			reader.On("FindSpans", mock.Anything, query).Return(spanSequence(chunks, nil)).Once()
			remote := spanRemote(t, reader)
			gotCaps, err := remote.SearchCapabilities(t.Context())
			require.NoError(t, err)
			assert.Equal(t, caps, gotCaps)
			got, err := jiter.CollectWithErrors(remote.FindSpans(t.Context(), query))
			require.NoError(t, err)
			assert.Equal(t, chunks, got)
			reader.AssertExpectations(t)
		})
	}
}

func TestSpanServerRefusesUnsupportedOrdering(t *testing.T) {
	for _, caps := range []tracestore.SearchCapabilities{{}, {SpanSearch: true}, {SpanSorting: true}} {
		reader := new(tracestoremocks.Reader)
		reader.On("SearchCapabilities", mock.Anything).Return(caps, nil)
		remote := spanRemote(t, reader)
		_, err := jiter.CollectWithErrors(remote.FindSpans(t.Context(), tracestore.SpanQueryParams{OrderBy: spanOrder()}))
		require.ErrorIs(t, err, tracestore.ErrSpanOrderInvalid)
		reader.AssertNotCalled(t, "FindSpans", mock.Anything, mock.Anything)
	}
	remote := NewTraceReader(startTestServer(t, &testServer{}))
	_, err := jiter.CollectWithErrors(remote.FindSpans(t.Context(), tracestore.SpanQueryParams{OrderBy: spanOrder()}))
	require.ErrorIs(t, err, errors.ErrUnsupported)
	_, err = jiter.CollectWithErrors(remote.FindSpans(t.Context(), tracestore.SpanQueryParams{}))
	require.ErrorIs(t, err, errors.ErrUnsupported)
}

func TestSpanRemoteErrors(t *testing.T) {
	for _, backendErr := range []error{errors.ErrUnsupported, tracestore.ErrPaginationInvalid, tracestore.ErrSpanOrderInvalid, status.Error(codes.Internal, "broken")} {
		reader := new(tracestoremocks.Reader)
		reader.On("FindSpans", mock.Anything, mock.Anything).Return(spanSequence(nil, backendErr))
		_, err := jiter.CollectWithErrors(spanRemote(t, reader).FindSpans(t.Context(), tracestore.SpanQueryParams{}))
		if status.Code(backendErr) == codes.Internal {
			assert.Equal(t, codes.Internal, status.Code(err))
		} else {
			require.ErrorIs(t, err, backendErr)
		}
	}
	reader := new(tracestoremocks.Reader)
	reader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{}, errors.New("offline"))
	_, err := jiter.CollectWithErrors(spanRemote(t, reader).FindSpans(t.Context(), tracestore.SpanQueryParams{OrderBy: spanOrder()}))
	require.ErrorContains(t, err, "offline")
	reader.AssertNotCalled(t, "FindSpans", mock.Anything, mock.Anything)
}

type spanStream struct {
	grpc.ServerStream
	sent []*storage.FindSpansResponse
	err  error
}

func (*spanStream) Context() context.Context { return context.Background() }
func (s *spanStream) Send(resp *storage.FindSpansResponse) error {
	s.sent = append(s.sent, resp)
	return s.err
}

func TestSpanServerRefusals(t *testing.T) {
	for _, capsErr := range []error{nil, errors.ErrUnsupported, errors.New("offline")} {
		reader := new(tracestoremocks.Reader)
		reader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{}, capsErr)
		wire, err := toProtoSpanQuery(tracestore.SpanQueryParams{OrderBy: spanOrder()})
		require.NoError(t, err)
		err = NewHandler(reader, nil, nil).FindSpans(&storage.FindSpansRequest{Query: wire}, &spanStream{})
		if capsErr != nil && !errors.Is(capsErr, errors.ErrUnsupported) {
			require.ErrorIs(t, err, capsErr)
		} else {
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
		}
		reader.AssertNotCalled(t, "FindSpans", mock.Anything, mock.Anything)
	}
	reader := new(tracestoremocks.Reader)
	handler := NewHandler(reader, nil, nil)
	err := handler.FindSpans(&storage.FindSpansRequest{}, &spanStream{})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	reader.On("FindSpans", mock.Anything, mock.Anything).Return(spanSequence([]tracestore.PageChunk[ptrace.Traces]{{Results: makeTestTrace()}}, nil))
	sendErr := errors.New("send failed")
	err = handler.FindSpans(&storage.FindSpansRequest{Query: &storage.SpanQueryParameters{}}, &spanStream{err: sendErr})
	require.ErrorIs(t, err, sendErr)
}

func TestSpanQueryConversion(t *testing.T) {
	filter := &expression.Call{Op: expression.OpEq, Args: []expression.Expression{&expression.FieldRef{Level: expression.LevelSpan, Name: "name"}, &expression.StringValue{Value: "operation"}}}
	query := tracestore.SpanQueryParams{Filter: filter, OrderBy: spanOrder()}
	wire, err := toProtoSpanQuery(query)
	require.NoError(t, err)
	got, err := toSpanQueryParams(wire)
	require.NoError(t, err)
	assert.Equal(t, query, got)
	for _, wire := range []*storage.SpanQueryParameters{
		nil,
		{OrderBy: []*storage.SpanSortOrder{nil}},
		{Filter: &expressionproto.Call{Args: []*expressionproto.Expression{nil}}},
	} {
		_, err := toSpanQueryParams(wire)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	}
	for _, query := range []tracestore.SpanQueryParams{
		{OrderBy: []tracestore.SpanSortOrder{{}}},
		{Filter: &expression.Call{Args: []expression.Expression{nil}}},
	} {
		remote := &TraceReader{}
		_, err := jiter.CollectWithErrors(remote.FindSpans(t.Context(), query))
		require.Error(t, err)
	}
}

func TestSpanRemoteEarlyExit(t *testing.T) {
	reader := new(tracestoremocks.Reader)
	reader.On("FindSpans", mock.Anything, mock.Anything).Return(spanSequence([]tracestore.PageChunk[ptrace.Traces]{{Results: makeTestTrace()}, {Results: makeTestTrace()}}, nil))
	count := 0
	for _, err := range spanRemote(t, reader).FindSpans(t.Context(), tracestore.SpanQueryParams{}) {
		require.NoError(t, err)
		count++
		break
	}
	assert.Equal(t, 1, count)
}

type failingSpanClient struct{ storage.TraceReaderClient }

func (failingSpanClient) FindSpans(context.Context, *storage.FindSpansRequest, ...grpc.CallOption) (storage.TraceReader_FindSpansClient, error) {
	return nil, status.Error(codes.Unavailable, "offline")
}

func TestSpanClientCannotStartStream(t *testing.T) {
	reader := &TraceReader{client: failingSpanClient{}}
	_, err := jiter.CollectWithErrors(reader.FindSpans(t.Context(), tracestore.SpanQueryParams{}))
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

func TestSpanQueryConversionDoesNotValidate(t *testing.T) {
	query := tracestore.SpanQueryParams{
		Filter:  &expression.Call{Op: "custom", Args: []expression.Expression{}},
		OrderBy: []tracestore.SpanSortOrder{{Expression: &expression.AttributeRef{Level: expression.LevelSpan, Key: "priority"}, Direction: "custom"}},
	}
	wire, err := toProtoSpanQuery(query)
	require.NoError(t, err)
	got, err := toSpanQueryParams(wire)
	require.NoError(t, err)
	assert.Equal(t, query, got)
}

func TestSpanHandlerValidatesBeforeStorage(t *testing.T) {
	for _, query := range []tracestore.SpanQueryParams{
		{Filter: &expression.Call{Op: "bad"}},
		{OrderBy: []tracestore.SpanSortOrder{{Expression: &expression.AttributeRef{Level: expression.LevelSpan, Key: "priority"}}}},
	} {
		wire, err := toProtoSpanQuery(query)
		require.NoError(t, err)
		reader := new(tracestoremocks.Reader)
		err = NewHandler(reader, nil, nil).FindSpans(&storage.FindSpansRequest{Query: wire}, &spanStream{})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		reader.AssertNotCalled(t, "FindSpans", mock.Anything, mock.Anything)
	}
}

type spanProxyServer struct {
	storage.UnimplementedTraceReaderServer
	requests chan *storage.FindSpansRequest
}

func (s *spanProxyServer) FindSpans(req *storage.FindSpansRequest, stream storage.TraceReader_FindSpansServer) error {
	s.requests <- req
	return stream.Send(&storage.FindSpansResponse{NextPageToken: "next"})
}

func TestFindSpansProxiesWithoutCapabilities(t *testing.T) {
	server := grpc.NewServer()
	peer := &spanProxyServer{requests: make(chan *storage.FindSpansRequest, 1)}
	storage.RegisterTraceReaderServer(server, peer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	reader := NewTraceReader(startServer(t, server, listener))
	query := tracestore.SpanQueryParams{
		OrderBy:    []tracestore.SpanSortOrder{{Expression: &expression.AttributeRef{Level: expression.LevelSpan, Key: "priority"}, Direction: "custom"}},
		Pagination: tracestore.Pagination{PageSize: 2, PageToken: "current"},
	}
	chunks, err := jiter.CollectWithErrors(reader.FindSpans(t.Context(), query))
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, tracestore.PageToken("next"), chunks[0].NextPageToken)
	forwarded, err := toSpanQueryParams((<-peer.requests).Query)
	require.NoError(t, err)
	assert.Equal(t, query, forwarded)
}
