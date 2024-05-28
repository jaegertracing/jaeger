// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package converter

import (
	"context"
	"encoding/binary"
	"fmt"

	otlp2jaeger "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"

	"github.com/jaegertracing/jaeger/model"
	spanstore_v1 "github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/spanstore"
)

type SpanReader struct {
	traceReader spanstore.Reader
}

func NewSpanReader(traceReader spanstore.Reader) (spanstore_v1.Reader, error) {
	return &SpanReader{
		traceReader: traceReader,
	}, nil
}

// FindTraceIDs implements spanstore.Reader.
func (s *SpanReader) FindTraceIDs(ctx context.Context, query *spanstore_v1.TraceQueryParameters) ([]model.TraceID, error) {
	IDs, err := s.traceReader.FindTraceIDs(ctx, spanstore.TraceQueryParameters{
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
		return []model.TraceID{}, err
	}

	traceIDs := []model.TraceID{}
	for _, ID := range IDs {
		// otelcol-contrib has the translator to jaeger proto but declared in private function
		// similar to https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/coreinternal/idutils/big_endian_converter.go#L21
		traceIDHigh, traceIDLow := binary.BigEndian.Uint64(ID[:8]), binary.BigEndian.Uint64(ID[8:])
		traceIDs = append(traceIDs, model.TraceID{
			Low:  traceIDLow,
			High: traceIDHigh,
		})
	}

	return traceIDs, nil
}

// FindTraces implements spanstore.Reader.
func (s *SpanReader) FindTraces(ctx context.Context, query *spanstore_v1.TraceQueryParameters) ([]*model.Trace, error) {
	tds, err := s.traceReader.FindTraces(ctx, spanstore.TraceQueryParameters{
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
		return []*model.Trace{}, err
	}

	traces := []*model.Trace{}
	for _, td := range tds {
		batches, err := otlp2jaeger.ProtoFromTraces(td)
		if err != nil {
			return []*model.Trace{}, fmt.Errorf("cannot transform OTLP trace to Jaeger format: %w", err)
		}

		trace := &model.Trace{}
		for _, batch := range batches {
			for _, span := range batch.Spans {
				if span.Process == nil {
					span.Process = batch.Process
				}
			}
			trace.Spans = append(trace.Spans, batch.Spans...)
		}
		traces = append(traces, trace)
	}

	return traces, nil
}

// GetOperations implements spanstore.Reader.
func (s *SpanReader) GetOperations(ctx context.Context, query spanstore_v1.OperationQueryParameters) ([]spanstore_v1.Operation, error) {
	ops, err := s.traceReader.GetOperations(ctx, spanstore.OperationQueryParameters{
		ServiceName: query.ServiceName,
		SpanKind:    query.SpanKind,
	})
	if err != nil {
		return []spanstore_v1.Operation{}, err
	}

	operations := []spanstore_v1.Operation{}
	for _, op := range ops {
		operations = append(operations, spanstore_v1.Operation{
			Name:     op.Name,
			SpanKind: op.SpanKind,
		})
	}
	return operations, nil
}

// GetServices implements spanstore.Reader.
func (s *SpanReader) GetServices(ctx context.Context) ([]string, error) {
	services, err := s.traceReader.GetServices(ctx)
	if err != nil {
		return []string{}, err
	}

	return services, nil
}

// GetTrace implements spanstore.Reader.
func (s *SpanReader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	// otelcol-contrib has the translator to pcommon.TraceID but declared in private function
	// similar to https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/coreinternal/idutils/big_endian_converter.go#L13
	ID := [16]byte{}
	binary.BigEndian.PutUint64(ID[:8], traceID.High)
	binary.BigEndian.PutUint64(ID[8:], traceID.Low)
	td, err := s.traceReader.GetTrace(ctx, ID)
	if err != nil {
		return nil, err
	}

	batches, err := otlp2jaeger.ProtoFromTraces(td)
	if err != nil {
		return nil, fmt.Errorf("cannot transform OTLP trace to Jaeger format: %w", err)
	}

	trace := &model.Trace{}
	for _, batch := range batches {
		for _, span := range batch.Spans {
			if span.Process == nil {
				span.Process = batch.Process
			}
		}
		trace.Spans = append(trace.Spans, batch.Spans...)
	}
	return trace, nil
}
