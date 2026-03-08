// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/jtracer"
	"github.com/jaegertracing/jaeger/internal/metrics"
	cascfg "github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	cspanstore "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var logger, _ = zap.NewDevelopment()

// TODO: this is going morph into a load testing framework for cassandra 3.7
func main() {
	noScope := metrics.NullFactory
	cConfig := &cascfg.Configuration{
		Schema: cascfg.Schema{
			Keyspace: "jaeger_v1_test",
		},
		Connection: cascfg.Connection{
			Servers:            []string{"127.0.0.1"},
			ConnectionsPerHost: 10,
			ProtoVersion:       4,
		},
		Query: cascfg.Query{
			Timeout: time.Millisecond * 750,
		},
	}
	cqlSession, err := cassandra.NewSession(cConfig)
	if err != nil {
		logger.Fatal("Cannot create Cassandra session", zap.Error(err))
	}
	tracerProvider, tracerCloser, err := jtracer.NewProvider(context.Background(), "savetracetest")
	if err != nil {
		logger.Fatal("Failed to initialize tracer", zap.Error(err))
	}
	defer tracerCloser(context.Background())
	spanStore, err := cspanstore.NewSpanWriter(cqlSession, time.Hour*12, noScope, logger)
	if err != nil {
		logger.Fatal("Failed to create span writer", zap.Error(err))
	}
	spanReader, err := cspanstore.NewSpanReader(cqlSession, noScope, logger, tracerProvider.Tracer("cspanstore.SpanReader"))
	if err != nil {
		logger.Fatal("Failed to create span reader", zap.Error(err))
	}
	ctx := context.Background()
	if err = spanStore.WriteSpan(ctx, getSomeSpan()); err != nil {
		logger.Fatal("Failed to save", zap.Error(err))
	} else {
		logger.Info("Saved span", zap.String("spanID", getSomeSpan().SpanID.String()))
	}
	s := getSomeSpan()
	tId := make([]byte, 0, 16)
	_, err = s.TraceID.MarshalTo(tId)
	if err != nil {
		logger.Fatal("Failed to marshal traceID", zap.Error(err))
	}
	var traceID [16]byte
	copy(traceID[:], tId)
	trace, err := spanReader.GetTrace(ctx, tracestore.GetTraceParams{TraceID: traceID})
	if err != nil {
		logger.Fatal("Failed to read", zap.Error(err))
	} else {
		logger.Info("Loaded trace", zap.Any("trace", trace))
	}

	tqp := &tracestore.TraceQueryParams{
		ServiceName:  "someServiceName",
		StartTimeMin: time.Now().Add(time.Hour * -1),
		StartTimeMax: time.Now().Add(time.Hour),
	}
	logger.Info("Check main query")
	queryAndPrint(ctx, spanReader, tqp)

	tqp.OperationName = "opName"
	logger.Info("Check query with operation")
	queryAndPrint(ctx, spanReader, tqp)

	tqp.Attributes = pcommon.NewMap()
	tqp.Attributes.PutStr("someKey", "someVal")
	logger.Info("Check query with operation name and tags")
	queryAndPrint(ctx, spanReader, tqp)

	tqp.DurationMin = 0
	tqp.DurationMax = time.Hour
	tqp.Attributes = pcommon.NewMap()
	logger.Info("check query with duration")
	queryAndPrint(ctx, spanReader, tqp)
}

func queryAndPrint(ctx context.Context, spanReader *cspanstore.SpanReader, tqp *tracestore.TraceQueryParams) {
	traces, err := spanReader.FindTraces(ctx, tqp)
	if err != nil {
		logger.Fatal("Failed to query", zap.Error(err))
	} else {
		logger.Info("Found trace(s)", zap.Any("traces", traces))
	}
}

func getSomeProcess() *model.Process {
	processTagVal := "indexMe"
	return &model.Process{
		ServiceName: "someServiceName",
		Tags: model.KeyValues{
			model.String("processTagKey", processTagVal),
		},
	}
}

func getSomeSpan() *model.Span {
	traceID := model.NewTraceID(1, 2)
	return &model.Span{
		TraceID:       traceID,
		SpanID:        model.NewSpanID(3),
		OperationName: "opName",
		References:    model.MaybeAddParentSpanID(traceID, 4, getReferences()),
		Flags:         model.Flags(uint32(5)),
		StartTime:     time.Now(),
		Duration:      50000 * time.Microsecond,
		Tags:          getTags(),
		Logs:          getLogs(),
		Process:       getSomeProcess(),
	}
}

func getReferences() []model.SpanRef {
	return []model.SpanRef{
		{
			RefType: model.ChildOf,
			TraceID: model.NewTraceID(1, 1),
			SpanID:  model.NewSpanID(4),
		},
	}
}

func getTags() model.KeyValues {
	someVal := "someVal"
	return model.KeyValues{
		model.String("someKey", someVal),
	}
}

func getLogs() []model.Log {
	logTag := "this is a msg"
	return []model.Log{
		{
			Timestamp: time.Now(),
			Fields: model.KeyValues{
				model.String("event", logTag),
			},
		},
	}
}
