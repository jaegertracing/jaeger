package v1adapter

import (
	"context"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

type SpanReader struct {
	traceReader tracestore.Reader
}

func NewSpanReader(traceReader tracestore.Reader) *SpanReader {
	return &SpanReader{
		traceReader: traceReader,
	}
}

func (sr *SpanReader) GetTrace(ctx context.Context, query spanstore.GetTraceParameters) (*model.Trace, error) {
	getTracesIter := sr.traceReader.GetTraces(ctx, tracestore.GetTraceParams{
		TraceID: query.TraceID.ToOTELTraceID(),
		Start:   query.StartTime,
		End:     query.EndTime,
	})
	traces, err := V1TracesFromSeq2(getTracesIter)
	if err != nil {
		return nil, err
	}
	if len(traces) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}
	return traces[0], nil
}

func (sr *SpanReader) GetServices(ctx context.Context) ([]string, error) {
	return sr.traceReader.GetServices(ctx)
}

func (sr *SpanReader) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	o, err := sr.traceReader.GetOperations(ctx, tracestore.OperationQueryParams{
		ServiceName: query.ServiceName,
		SpanKind:    query.SpanKind,
	})
	if err != nil || o == nil {
		return nil, err
	}
	operations := []spanstore.Operation{}
	for _, operation := range o {
		operations = append(operations, spanstore.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		})
	}
	return operations, nil
}

func (sr *SpanReader) FindTraces(
	ctx context.Context,
	query *spanstore.TraceQueryParameters,
) ([]*model.Trace, error) {
	getTracesIter := sr.traceReader.FindTraces(ctx, tracestore.TraceQueryParams{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		Tags:          query.Tags,
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		NumTraces:     query.NumTraces,
	})
	return V1TracesFromSeq2(getTracesIter)
}

func (sr *SpanReader) FindTraceIDs(
	ctx context.Context,
	query *spanstore.TraceQueryParameters,
) ([]model.TraceID, error) {
	traceIDsIter := sr.traceReader.FindTraceIDs(ctx, tracestore.TraceQueryParams{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		Tags:          query.Tags,
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		NumTraces:     query.NumTraces,
	})
	var (
		iterErr       error
		modelTraceIDs []model.TraceID
	)
	traceIDsIter(func(traceIDs []pcommon.TraceID, err error) bool {
		if err != nil {
			iterErr = err
			return false
		}
		for _, traceID := range traceIDs {
			model.TraceIDFromOTEL(traceID)
			modelTraceIDs = append(modelTraceIDs, model.TraceIDFromOTEL(traceID))
		}
		return true
	})
	if iterErr != nil {
		return nil, iterErr
	}
	return modelTraceIDs, nil
}
