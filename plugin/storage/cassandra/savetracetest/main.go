// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	cascfg "github.com/jaegertracing/jaeger/pkg/cassandra/config"
	cSpanStore "github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var logger, _ = zap.NewDevelopment()

// TODO: this is going morph into a load testing framework for cassandra 3.7
func main() {
	noScope := metrics.NullFactory
	cConfig := &cascfg.Configuration{
		Servers:            []string{"127.0.0.1"},
		ConnectionsPerHost: 10,
		Timeout:            time.Millisecond * 750,
		ProtoVersion:       4,
		Keyspace:           "jaeger_v1_test",
	}
	cqlSession, err := cConfig.NewSession(logger)
	if err != nil {
		logger.Fatal("Cannot create Cassandra session", zap.Error(err))
	}
	spanStore := cSpanStore.NewSpanWriter(cqlSession, time.Hour*12, noScope, logger)
	spanReader := cSpanStore.NewSpanReader(cqlSession, noScope, logger)
	ctx := context.Background()
	if err = spanStore.WriteSpan(ctx, getSomeSpan()); err != nil {
		logger.Fatal("Failed to save", zap.Error(err))
	} else {
		logger.Info("Saved span", zap.String("spanID", getSomeSpan().SpanID.String()))
	}
	s := getSomeSpan()
	trace, err := spanReader.GetTrace(ctx, s.TraceID)
	if err != nil {
		logger.Fatal("Failed to read", zap.Error(err))
	} else {
		logger.Info("Loaded trace", zap.Any("trace", trace))
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

func queryAndPrint(ctx context.Context, spanReader *cSpanStore.SpanReader, tqp *spanstore.TraceQueryParameters) {
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
