// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"strings"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/ports"
)

var (
	_ tracestore.Reader = (*traceReader)(nil)
	_ io.Closer         = (*traceReader)(nil)
)

// traceReader retrieves trace data from the jaeger-v2 query service through the api_v3.QueryServiceClient.
type traceReader struct {
	logger       *zap.Logger
	clientConn   *grpc.ClientConn
	client       api_v3.QueryServiceClient
	capabilities api_v3.CapabilitiesClient
}

// SearchCapabilities asks the query service what the storage behind it declares, through the
// api_v3 Capabilities service. A query service whose storage cannot report answers
// UNIMPLEMENTED, which becomes ErrUnsupported so the caller reads it as the least capable
// backend.
func (r *traceReader) SearchCapabilities(ctx context.Context) (tracestore.SearchCapabilities, error) {
	resp, err := r.capabilities.GetCapabilities(ctx, &api_v3.GetCapabilitiesRequest{})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return tracestore.SearchCapabilities{}, fmt.Errorf(
				"the query service does not report its storage's search capabilities: %w", errors.ErrUnsupported,
			)
		}
		return tracestore.SearchCapabilities{}, err
	}
	search := resp.GetSearch()
	return tracestore.SearchCapabilities{
		WithoutServiceName:  search.GetWithoutServiceName(),
		SameSpanConjunction: search.GetSameSpanConjunction(),
		Filter:              fromAPIFilterCapabilities(search.GetFilter()),
		Paginated:           search.GetPaginated(),
		SpanSearch:          search.GetSpanSearch(),
		SpanSorting:         search.GetSpanSorting(),
	}, nil
}

func fromAPIFilterCapabilities(caps *api_v3.FilterCapabilities) *tracestore.FilterCapabilities {
	if caps == nil {
		return nil
	}
	levels := make([]expression.Level, 0, len(caps.GetLevels()))
	for _, level := range caps.GetLevels() {
		levels = append(levels, expression.Level(level))
	}
	operators := make([]expression.Operator, 0, len(caps.GetOperators()))
	for _, op := range caps.GetOperators() {
		operators = append(operators, expression.Operator(op))
	}
	return &tracestore.FilterCapabilities{
		Levels:    levels,
		Operators: operators,
	}
}

func createTraceReader(logger *zap.Logger, port int) (*traceReader, error) {
	logger.Info("Creating the trace reader", zap.Int("port", port))
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	cc, err := grpc.NewClient(ports.PortToHostPort(port), opts...)
	if err != nil {
		return nil, err
	}

	return &traceReader{
		logger:       logger,
		clientConn:   cc,
		client:       api_v3.NewQueryServiceClient(cc),
		capabilities: api_v3.NewCapabilitiesClient(cc),
	}, nil
}

func (r *traceReader) Close() error {
	r.logger.Info("Closing the gRPC trace reader")
	return r.clientConn.Close()
}

func (r *traceReader) GetTraces(
	ctx context.Context,
	traceIDs ...tracestore.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		// api_v3 does not support multi-get, so loop through IDs
		for _, idParams := range traceIDs {
			idStr := v1adapter.ToV1TraceID(idParams.TraceID).String()
			r.logger.Info("Calling api_v3.GetTrace", zap.String("trace_id", idStr))
			stream, err := r.client.GetTrace(ctx, &api_v3.GetTraceRequest{
				TraceId:   idStr,
				StartTime: idParams.Start,
				EndTime:   idParams.End,
			})
			if !r.consumeTraces(yield, stream, err) {
				return
			}
		}
	}
}

func (r *traceReader) GetServices(ctx context.Context) ([]string, error) {
	res, err := r.client.GetServices(ctx, &api_v3.GetServicesRequest{})
	if err != nil {
		return []string{}, err
	}
	return res.Services, nil
}

func (r *traceReader) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	var operations []tracestore.Operation
	res, err := r.client.GetOperations(ctx, &api_v3.GetOperationsRequest{
		Service:  query.ServiceName,
		SpanKind: query.SpanKind,
	})
	if err != nil {
		return operations, err
	}
	for _, operation := range res.Operations {
		operations = append(operations, tracestore.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		})
	}
	return operations, nil
}

// toProtoQuery renders a search as the api_v3 request the query service reads, which is the
// only shape the e2e tests can send: they drive jaeger over the wire rather than calling a
// Reader, so a predicate this function drops is one no e2e test can exercise.
func toProtoQuery(query tracestore.TraceQueryParams) (*api_v3.TraceQueryParameters, error) {
	if query.SearchDepth > math.MaxInt32 {
		return nil, fmt.Errorf("SearchDepth must not be greater than %d", math.MaxInt32)
	}
	protoQuery := &api_v3.TraceQueryParameters{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		Attributes:    jptrace.PcommonMapToPlainMap(query.Attributes),
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		SearchDepth:   int32(query.SearchDepth),
	}
	if query.Pagination != nil {
		protoQuery.Pagination = &api_v3.Pagination{
			PageSize:  query.Pagination.PageSize,
			PageToken: string(query.Pagination.PageToken),
		}
	}
	if query.Filter != nil {
		filter, err := expressionproto.CallToProto(query.Filter)
		if err != nil {
			return nil, fmt.Errorf("cannot encode the query filter: %w", err)
		}
		protoQuery.Filter = filter
	}
	return protoQuery, nil
}

func (r *traceReader) FindTraces(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		protoQuery, err := toProtoQuery(query)
		if err != nil {
			yield(nil, err)
			return
		}
		protoQuery.RawTraces = true
		stream, err := r.client.FindTraces(ctx, &api_v3.FindTracesRequest{Query: protoQuery})
		r.consumeTraces(yield, stream, err)
	}
}

func (*traceReader) FindTraceIDs(
	_ context.Context,
	_ tracestore.TraceQueryParams,
) iter.Seq2[tracestore.PageChunk[[]tracestore.FoundTraceID], error] {
	panic("not implemented")
}

func (r *traceReader) FindSpans(ctx context.Context, query tracestore.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
		filter, err := expressionproto.CallToProto(query.Filter)
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		var terms []*api_v3.SpanSortOrder
		for _, term := range query.OrderBy {
			encoded, err := expressionproto.ToProto(term.Expression)
			if err != nil {
				yield(tracestore.PageChunk[ptrace.Traces]{}, err)
				return
			}
			terms = append(terms, &api_v3.SpanSortOrder{Expression: encoded, Direction: string(term.Direction)})
		}
		stream, err := r.client.FindSpans(ctx, &api_v3.FindSpansRequest{Query: &api_v3.SpanQueryParameters{
			StartTimeMin: query.StartTimeMin,
			StartTimeMax: query.StartTimeMax,
			Filter:       filter,
			OrderBy:      terms,
			Pagination: &api_v3.Pagination{
				PageSize:  query.Pagination.PageSize,
				PageToken: string(query.Pagination.PageToken),
			},
		}})
		if err != nil {
			yield(tracestore.PageChunk[ptrace.Traces]{}, err)
			return
		}
		for {
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				yield(tracestore.PageChunk[ptrace.Traces]{}, err)
				return
			}
			spans := ptrace.NewTraces()
			if response.Spans != nil {
				spans = response.Spans.ToTraces()
			}
			if !yield(tracestore.PageChunk[ptrace.Traces]{Results: spans, NextPageToken: tracestore.PageToken(response.NextPageToken)}, nil) {
				return
			}
		}
	}
}

func (r *traceReader) FindTraceSummaries(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[tracestore.PageChunk[[]tracestore.TraceSummary], error] {
	return func(yield func(tracestore.PageChunk[[]tracestore.TraceSummary], error) bool) {
		protoQuery, err := toProtoQuery(query)
		if err != nil {
			yield(tracestore.PageChunk[[]tracestore.TraceSummary]{}, err)
			return
		}
		stream, err := r.client.FindTraceSummaries(ctx, &api_v3.FindTraceSummariesRequest{Query: protoQuery})
		if err != nil {
			if status.Code(err) == codes.Unimplemented {
				err = fmt.Errorf("remote server does not support FindTraceSummaries: %w", errors.ErrUnsupported)
			}
			yield(tracestore.PageChunk[[]tracestore.TraceSummary]{}, err)
			return
		}
		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				yield(tracestore.PageChunk[[]tracestore.TraceSummary]{}, err)
				return
			}
			batch := make([]tracestore.TraceSummary, len(resp.GetSummaries()))
			for i, ps := range resp.GetSummaries() {
				traceID, parseErr := jptrace.TraceIDFromString(ps.GetTraceId())
				if parseErr != nil {
					yield(tracestore.PageChunk[[]tracestore.TraceSummary]{}, parseErr)
					return
				}
				svcs := make([]tracestore.ServiceSummary, len(ps.GetServices()))
				for j, ss := range ps.GetServices() {
					svcs[j] = tracestore.ServiceSummary{
						Name:           ss.GetName(),
						SpanCount:      int(ss.GetSpanCount()),
						ErrorSpanCount: int(ss.GetErrorSpanCount()),
					}
				}
				batch[i] = tracestore.TraceSummary{
					TraceID:           traceID,
					RootServiceName:   ps.GetRootServiceName(),
					RootOperationName: ps.GetRootOperationName(),
					MinStartTime:      jptrace.UnixNanoToTime(ps.GetMinStartTimeUnixNano()),
					MaxEndTime:        jptrace.UnixNanoToTime(ps.GetMaxEndTimeUnixNano()),
					SpanCount:         int(ps.GetSpanCount()),
					ErrorSpanCount:    int(ps.GetErrorSpanCount()),
					OrphanSpanCount:   int(ps.GetOrphanSpanCount()),
					Services:          svcs,
				}
			}
			chunk := tracestore.PageChunk[[]tracestore.TraceSummary]{
				Results:       batch,
				NextPageToken: tracestore.PageToken(resp.GetNextPageToken()),
			}
			if !yield(chunk, nil) {
				return
			}
		}
	}
}

type traceStream interface {
	Recv() (*jptrace.TracesData, error)
}

// apiV3ErrorInfoDomain is the ErrorInfo domain the api_v3 gRPC handler stamps on a refusal,
// which is what lets tracestore.ErrorFromStatus restore the reader's sentinel on this side.
const apiV3ErrorInfoDomain = "jaeger.api_v3"

// consumeTraces reads the stream and calls yield for each chunk.
// It also handles NotFound errors by terminating the stream.
// It returns false if the processing was terminated through error.
func (r *traceReader) consumeTraces(
	yield func([]ptrace.Traces, error) bool,
	stream traceStream,
	startErr error,
) bool {
	handleError := func(err error) bool {
		if err == nil {
			return true
		}
		err = unwrapNotFoundErr(err)
		// A refusal crosses the api_v3 hop as a status, so the shared suite can assert on the
		// same error family a direct reader returns (ADR-013).
		err = tracestore.ErrorFromStatus(err, apiV3ErrorInfoDomain)
		r.logger.Info("Error received", zap.Error(err))
		if !errors.Is(err, spanstore.ErrTraceNotFound) {
			yield(nil, err)
		}
		return false
	}
	if !handleError(startErr) {
		return false
	}
	for chunk, err := stream.Recv(); !errors.Is(err, io.EOF); chunk, err = stream.Recv() {
		if !handleError(err) {
			return false
		}
		traces := chunk.ToTraces() // unwrap ptrace.Traces from chunk
		if !yield([]ptrace.Traces{traces}, nil) {
			return false
		}
	}
	return true
}

func unwrapNotFoundErr(err error) error {
	if s, _ := status.FromError(err); s != nil {
		if strings.Contains(s.Message(), spanstore.ErrTraceNotFound.Error()) {
			return spanstore.ErrTraceNotFound
		}
	}
	return err
}
