// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/jtracer"
	"github.com/jaegertracing/jaeger/internal/metrics"
	cascfg "github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	cspanstore "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
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
	if err = spanStore.WriteSpan(getSomeSpan()); err != nil {
		logger.Fatal("Failed to save", zap.Error(err))
	} else {
		logger.Info("Saved span", zap.String("spanID", strconv.FormatInt(getSomeSpan().SpanID, 10)))
	}
	s := getSomeSpan()
	spans, err := spanReader.GetTrace(ctx, s.TraceID)
	if err != nil {
		logger.Fatal("Failed to read", zap.Error(err))
	} else {
		logger.Info("Loaded trace", zap.Any("spans", spans))
	}

	tqp := &spanstore.TraceQueryParameters{
		ServiceName:  "someServiceName",
		StartTimeMin: time.Now().Add(time.Hour * -1),
		StartTimeMax: time.Now().Add(time.Hour),
	}
	logger.Info("Check main query")
	queryAndPrint(ctx, spanReader, tqp)

	tqp.OperationName = "opName"
	logger.Info("Check query with operation")
	queryAndPrint(ctx, spanReader, tqp)

	tqp.Tags = map[string]string{
		"someKey": "someVal",
	}
	logger.Info("Check query with operation name and tags")
	queryAndPrint(ctx, spanReader, tqp)

	tqp.DurationMin = 0
	tqp.DurationMax = time.Hour
	tqp.Tags = map[string]string{}
	logger.Info("check query with duration")
	queryAndPrint(ctx, spanReader, tqp)
}

func queryAndPrint(ctx context.Context, spanReader *cspanstore.SpanReader, tqp *spanstore.TraceQueryParameters) {
	var traces []dbmodel.Trace
	var err error
	for trace, inerr := range spanReader.FindTraces(ctx, tqp) {
		if inerr != nil {
			err = inerr
			break
		}
		traces = append(traces, trace)
	}
	if err != nil {
		logger.Fatal("Failed to query", zap.Error(err))
	} else {
		logger.Info("Found trace(s)", zap.Any("traces", traces))
	}
}

func getSomeProcess() dbmodel.Process {
	processTagVal := "indexMe"
	return dbmodel.Process{
		ServiceName: "someServiceName",
		Tags: []dbmodel.KeyValue{
			{
				Key:         "processTagKey",
				ValueString: processTagVal,
			},
		},
	}
}

func getSomeSpan() *dbmodel.Span {
	traceID := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	return &dbmodel.Span{
		TraceID:       traceID,
		SpanID:        3,
		OperationName: "opName",
		Refs:          getReferences(),
		Flags:         5,
		StartTime:     time.Now().UnixMicro(),
		Duration:      (50000 * time.Microsecond).Microseconds(),
		Tags:          getTags(),
		Logs:          getLogs(),
		Process:       getSomeProcess(),
	}
}

func getReferences() []dbmodel.SpanRef {
	return []dbmodel.SpanRef{
		{
			RefType: dbmodel.ChildOf,
			TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 1},
			SpanID:  4,
		},
	}
}

func getTags() []dbmodel.KeyValue {
	someVal := "someVal"
	return []dbmodel.KeyValue{
		{
			Key:         "someKey",
			ValueString: someVal,
		},
	}
}

func getLogs() []dbmodel.Log {
	logTag := "this is a msg"
	return []dbmodel.Log{
		{
			Timestamp: time.Now().UnixMicro(),
			Fields: []dbmodel.KeyValue{
				{
					Key:         "event",
					ValueString: logTag,
				},
			},
		},
	}
}
