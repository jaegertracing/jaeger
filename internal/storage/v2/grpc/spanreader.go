// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	storage "github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func (tr *TraceReader) FindSpans(ctx context.Context, params tracestore.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
		query, err := toProtoSpanQuery(params)
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		if len(params.OrderBy) > 0 {
			caps, err := tr.SearchCapabilities(ctx)
			if err != nil && !errors.Is(err, errors.ErrUnsupported) {
				yield(tracestore.PageChunk[ptrace.Traces]{}, err)
				return
			}
			if err := caps.ValidateSpanSorting(params.OrderBy); err != nil {
				yield(tracestore.PageChunk[ptrace.Traces]{}, err)
				return
			}
		}
		stream, err := tr.client.FindSpans(ctx, &storage.FindSpansRequest{Query: query})
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, fmt.Errorf("failed to execute FindSpans: %w", spanReaderError(err)))
			return
		}
		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				yield(tracestore.PageChunk[ptrace.Traces]{}, spanReaderError(err))
				return
			}
			traces := ptrace.NewTraces()
			if resp.Spans != nil {
				traces = resp.Spans.ToTraces()
			}
			if !yield(tracestore.PageChunk[ptrace.Traces]{Results: traces, NextPageToken: tracestore.PageToken(resp.NextPageToken)}, nil) {
				return
			}
		}
	}
}

func spanReaderError(err error) error {
	err = readerError(err)
	if status.Code(err) == codes.Unimplemented {
		return fmt.Errorf("FindSpans: %w", errors.ErrUnsupported)
	}
	return err
}

func toProtoSpanQuery(params tracestore.SpanQueryParams) (*storage.SpanQueryParameters, error) {
	order, err := tracestore.NormalizeSpanOrder(params.OrderBy)
	if err != nil {
		return nil, err
	}
	filter, err := expressionproto.ToProto(params.Filter)
	if err != nil {
		return nil, err
	}
	query := &storage.SpanQueryParameters{
		StartTimeMin: params.StartTimeMin, StartTimeMax: params.StartTimeMax, Filter: filter,
		Pagination: &storage.Pagination{PageSize: params.Pagination.PageSize, PageToken: string(params.Pagination.PageToken)},
	}
	for _, term := range order {
		encoded, err := expressionproto.ExpressionToProto(term.Expression)
		if err != nil {
			return nil, err
		}
		query.OrderBy = append(query.OrderBy, &storage.SpanSortOrder{Expression: encoded, Direction: string(term.Direction)})
	}
	return query, nil
}

func toSpanQueryParams(wire *storage.SpanQueryParameters) (tracestore.SpanQueryParams, error) {
	if wire == nil {
		return tracestore.SpanQueryParams{}, status.Error(codes.InvalidArgument, "missing query")
	}
	order, err := tracestore.SpanOrderFromProto(wire.OrderBy)
	if err != nil {
		return tracestore.SpanQueryParams{}, status.Error(codes.InvalidArgument, err.Error())
	}
	filter, err := expressionproto.FromProto(wire.Filter)
	if err == nil && filter != nil {
		filter, err = tracestore.FinalizeFilter(filter)
	}
	if err != nil {
		return tracestore.SpanQueryParams{}, status.Error(codes.InvalidArgument, err.Error())
	}
	return tracestore.SpanQueryParams{
		StartTimeMin: wire.StartTimeMin, StartTimeMax: wire.StartTimeMax, Filter: filter, OrderBy: order,
		Pagination: tracestore.Pagination{PageSize: wire.GetPagination().GetPageSize(), PageToken: tracestore.PageToken(wire.GetPagination().GetPageToken())},
	}, nil
}

func (h *Handler) FindSpans(req *storage.FindSpansRequest, srv storage.TraceReader_FindSpansServer) error {
	query, err := toSpanQueryParams(req.GetQuery())
	if err != nil {
		return err
	}
	if len(query.OrderBy) > 0 {
		caps, err := h.traceReader.SearchCapabilities(srv.Context())
		if err != nil && !errors.Is(err, errors.ErrUnsupported) {
			return err
		}
		if err := caps.ValidateSpanSorting(query.OrderBy); err != nil {
			return readerStatus(err)
		}
	}
	for chunk, err := range h.traceReader.FindSpans(srv.Context(), query) {
		if err != nil {
			if errors.Is(err, errors.ErrUnsupported) {
				return status.Error(codes.Unimplemented, err.Error())
			}
			return readerStatus(err)
		}
		data := jptrace.TracesData(chunk.Results)
		if err := srv.Send(&storage.FindSpansResponse{Spans: &data, NextPageToken: string(chunk.NextPageToken)}); err != nil {
			return err
		}
	}
	return nil
}
