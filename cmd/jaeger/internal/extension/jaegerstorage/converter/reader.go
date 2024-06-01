// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package converter

import (
	"context"
	"encoding/binary"
	"fmt"

	jaeger2otlp "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	spanstore_v1 "github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/spanstore"
)

type TraceReader struct {
	spanReader spanstore_v1.Reader
}

func NewTraceReader(spanReader spanstore_v1.Reader) (spanstore.Reader, error) {
	return &TraceReader{
		spanReader: spanReader,
	}, nil
}

// GetTrace implements spanstore.Reader.
func (s *TraceReader) GetTrace(ctx context.Context, traceID pcommon.TraceID) (ptrace.Traces, error) {
	// otelcol-contrib has the translator to jaeger proto but declared in private function
	// similar to https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/coreinternal/idutils/big_endian_converter.go#L21
	traceIDHigh, traceIDLow := binary.BigEndian.Uint64(traceID[:8]), binary.BigEndian.Uint64(traceID[8:])
	ID := model.TraceID{
		Low:  traceIDLow,
		High: traceIDHigh,
	}
	trace, err := s.spanReader.GetTrace(ctx, ID)
	if err != nil {
		return ptrace.NewTraces(), err
	}

	batches := []*model.Batch{{Spans: trace.Spans}}
	td, err := jaeger2otlp.ProtoToTraces(batches)
	if err != nil {
		return ptrace.NewTraces(), fmt.Errorf("cannot transform Jaeger trace to OTLP format: %w", err)
	}

	return td, nil
}

// GetServices implements spanstore.Reader.
func (s *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	services, err := s.spanReader.GetServices(ctx)
	if err != nil {
		return []string{}, err
	}
	return services, nil
}

// GetOperations implements spanstore.Reader.
func (s *TraceReader) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	ops, err := s.spanReader.GetOperations(ctx, spanstore_v1.OperationQueryParameters{
		ServiceName: query.ServiceName,
		SpanKind:    query.SpanKind,
	})
	if err != nil {
		return []spanstore.Operation{}, err
	}

	operations := []spanstore.Operation{}
	for _, op := range ops {
		operations = append(operations, spanstore.Operation{
			Name:     op.Name,
			SpanKind: op.SpanKind,
		})
	}
	return operations, nil
}

// FindTraces implements spanstore.Reader.
func (s *TraceReader) FindTraces(ctx context.Context, query spanstore.TraceQueryParameters) ([]ptrace.Traces, error) {
	traces, err := s.spanReader.FindTraces(ctx, &spanstore_v1.TraceQueryParameters{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		Tags:          query.Tags,
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		NumTraces:     query.NumTraces,
	})
	if err != nil {
		return []ptrace.Traces{}, err
	}

	tds := []ptrace.Traces{}
	for _, trace := range traces {
		batch := []*model.Batch{{Spans: trace.Spans}}
		td, err := jaeger2otlp.ProtoToTraces(batch)
		if err != nil {
			return []ptrace.Traces{}, fmt.Errorf("cannot transform Jaeger trace to OTLP format: %w", err)
		}

		tds = append(tds, td)
	}

	return tds, nil
}

// FindTraceIDs implements spanstore.Reader.
func (s *TraceReader) FindTraceIDs(ctx context.Context, query spanstore.TraceQueryParameters) ([]pcommon.TraceID, error) {
	IDs, err := s.spanReader.FindTraceIDs(ctx, &spanstore_v1.TraceQueryParameters{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		Tags:          query.Tags,
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		NumTraces:     query.NumTraces,
	})
	if err != nil {
		return []pcommon.TraceID{}, err
	}

	traceIDs := []pcommon.TraceID{}
	for _, ID := range IDs {
		// otelcol-contrib has the translator to OTLP but declared in private function
		// similar to https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/coreinternal/idutils/big_endian_converter.go#L13
		traceID := [16]byte{}
		binary.BigEndian.PutUint64(traceID[:8], ID.High)
		binary.BigEndian.PutUint64(traceID[8:], ID.Low)
		traceIDs = append(traceIDs, traceID)
	}

	return traceIDs, nil
}
