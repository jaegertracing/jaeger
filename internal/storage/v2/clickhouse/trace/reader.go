// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package trace

import (
	"context"
	"errors"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/trace/dbmodel"
)

const querySQL = `
SELECT
    Timestamp,
    TraceId,
    SpanId,
    ParentSpanId,
    TraceState,
    SpanName,
    SpanKind,
    ServiceName,
    ResourceAttributes.keys,
    ResourceAttributes.values,
    ScopeName,
    ScopeVersion,
    SpanAttributes.keys,
	SpanAttributes.values,
    Duration,
    StatusCode,
    StatusMessage,
    Events.Timestamp,
    Events.Name,
    Events.Attributes,
    Links.TraceId,
    Links.SpanId,
    Links.TraceState,
    Links.Attributes,
FROM otel_traces
WHERE TraceId = ?;
`

type Reader struct {
	Client client.Conn
}

func (Reader) GetServices(_ context.Context) ([]string, error) {
	panic("implement me")
}

func (Reader) GetOperations(_ context.Context, _ tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	panic("implement me")
}

func (Reader) FindTraces(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	panic("implement me")
}

func (Reader) FindTraceIDs(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	panic("implement me")
}

func (tr Reader) GetTraces(
	ctx context.Context,
	traceIDs ...tracestore.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		for _, id := range traceIDs {
			tds, err := tr.getTraces(ctx, querySQL, id)
			if err != nil {
				if errors.Is(err, errors.New("trace not found")) {
					continue
				}
				yield(nil, err)
				return
			}
			if !yield(tds, nil) {
				return
			}
		}
	}
}

func NewTraceReader(c client.Conn) (*Reader, error) {
	if c == nil {
		return nil, errors.New("can't create trace reader with nil clickhouse")
	}
	return &Reader{Client: c}, nil
}

func (tr *Reader) getTraces(ctx context.Context, query string, param tracestore.GetTraceParams) ([]ptrace.Traces, error) {
	rows, err := tr.Client.Query(ctx, query, param.TraceID.String())
	if err != nil {
		return nil, err
	}
	pts := make([]ptrace.Traces, 0)
	for rows.Next() {
		var dbTrace dbmodel.Model
		err := rows.Scan(
			&dbTrace.Timestamp,
			&dbTrace.TraceId,
			&dbTrace.SpanId,
			&dbTrace.ParentSpanId,
			&dbTrace.TraceState,
			&dbTrace.SpanName,
			&dbTrace.SpanKind,
			&dbTrace.ServiceName,
			&dbTrace.ResourceAttributesKeys,
			&dbTrace.ResourceAttributesValues,
			&dbTrace.ScopeName,
			&dbTrace.ScopeVersion,
			&dbTrace.SpanAttributesKeys,
			&dbTrace.SpanAttributesValues,
			&dbTrace.Duration,
			&dbTrace.StatusCode,
			&dbTrace.StatusMessage,
			&dbTrace.EventsTimestamp,
			&dbTrace.EventsName,
			&dbTrace.EventsAttributes,
			&dbTrace.LinksTraceId,
			&dbTrace.LinksSpanId,
			&dbTrace.LinksTraceState,
			&dbTrace.LinksAttributes,
		)
		if err != nil {
			return pts, err
		}
		pt, err := dbTrace.ConvertToTraces()
		if err != nil {
			return pts, err
		}
		pts = append(pts, pt)
	}
	return pts, nil
}
