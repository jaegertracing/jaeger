// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/ports"
)

var (
	_ tracestore.Reader = (*traceReader)(nil)
	_ io.Closer         = (*traceReader)(nil)
)

// traceReader retrieves trace data from the jaeger-v2 query service through the api_v2.QueryServiceClient.
type traceReader struct {
	logger     *zap.Logger
	clientConn *grpc.ClientConn
	client     api_v3.QueryServiceClient
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
		logger:     logger,
		clientConn: cc,
		client:     api_v3.NewQueryServiceClient(cc),
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

func (r *traceReader) FindTraces(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		if query.SearchDepth > math.MaxInt32 {
			yield(nil, fmt.Errorf("NumTraces must not be greater than %d", math.MaxInt32))
			return
		}
		stream, err := r.client.FindTraces(ctx, &api_v3.FindTracesRequest{
			Query: &api_v3.TraceQueryParameters{
				ServiceName:   query.ServiceName,
				OperationName: query.OperationName,
				Attributes:    jptrace.PcommonMapToPlainMap(query.Attributes),
				StartTimeMin:  query.StartTimeMin,
				StartTimeMax:  query.StartTimeMax,
				DurationMin:   query.DurationMin,
				DurationMax:   query.DurationMax,
				SearchDepth:   int32(query.SearchDepth), //nolint:gosec // G115 - bounds checked above
				RawTraces:     true,
			},
		})
		r.consumeTraces(yield, stream, err)
	}
}

func (*traceReader) FindTraceIDs(
	_ context.Context,
	_ tracestore.TraceQueryParams,
) iter.Seq2[[]tracestore.FoundTraceID, error] {
	panic("not implemented")
}

func (r *traceReader) FindTraceSummaries(
	ctx context.Context,
	query tracestore.TraceQueryParams,
) iter.Seq2[[]tracestore.TraceSummary, error] {
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		if query.SearchDepth > math.MaxInt32 {
			yield(nil, fmt.Errorf("SearchDepth must not be greater than %d", math.MaxInt32))
			return
		}
		stream, err := r.client.FindTraceSummaries(ctx, &api_v3.FindTraceSummariesRequest{
			Query: &api_v3.TraceQueryParameters{
				ServiceName:   query.ServiceName,
				OperationName: query.OperationName,
				Attributes:    jptrace.PcommonMapToPlainMap(query.Attributes),
				StartTimeMin:  query.StartTimeMin,
				StartTimeMax:  query.StartTimeMax,
				DurationMin:   query.DurationMin,
				DurationMax:   query.DurationMax,
				SearchDepth:   int32(query.SearchDepth), //nolint:gosec // G115 - bounds checked above
			},
		})
		if err != nil {
			if status.Code(err) == codes.Unimplemented {
				err = fmt.Errorf("remote server does not support FindTraceSummaries: %w", errors.ErrUnsupported)
			}
			yield(nil, err)
			return
		}
		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				yield(nil, err)
				return
			}
			batch := make([]tracestore.TraceSummary, len(resp.GetSummaries()))
			for i, ps := range resp.GetSummaries() {
				traceID, parseErr := traceIDFromHex(ps.GetTraceId())
				if parseErr != nil {
					yield(nil, parseErr)
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
			if !yield(batch, nil) {
				return
			}
		}
	}
}

type traceStream interface {
	Recv() (*jptrace.TracesData, error)
}

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

// traceIDFromHex parses a 32-character hex string into a pcommon.TraceID.
func traceIDFromHex(s string) (pcommon.TraceID, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return pcommon.TraceID{}, fmt.Errorf("invalid trace ID %q: %w", s, err)
	}
	if len(b) != 16 {
		return pcommon.TraceID{}, fmt.Errorf("trace ID must be 16 bytes, got %d", len(b))
	}
	return pcommon.TraceID(b), nil
}
