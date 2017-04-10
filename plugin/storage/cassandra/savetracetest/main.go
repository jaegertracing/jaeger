// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package main

import (
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	cascfg "github.com/uber/jaeger/pkg/cassandra/config"
	cSpanStore "github.com/uber/jaeger/plugin/storage/cassandra/spanstore"
	"github.com/uber/jaeger/storage/spanstore"
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
	cqlSession, err := cConfig.NewSession()
	if err != nil {
		logger.Fatal("Cannot create Cassandra session", zap.Error(err))
	}
	spanStore := cSpanStore.NewSpanWriter(cqlSession, time.Hour*12, noScope, logger)
	spanReader := cSpanStore.NewSpanReader(cqlSession, noScope, logger)
	if err = spanStore.WriteSpan(getSomeSpan()); err != nil {
		logger.Fatal("Failed to save", zap.Error(err))
	} else {
		logger.Info("Saved span", zap.String("spanID", getSomeSpan().SpanID.String()))
	}
	s := getSomeSpan()
	trace, err := spanReader.GetTrace(s.TraceID)
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
	queryAndPrint(spanReader, tqp)

	tqp.OperationName = "opName"
	logger.Info("Check query with operation")
	queryAndPrint(spanReader, tqp)

	tqp.Tags = map[string]string{
		"someKey": "someVal",
	}
	logger.Info("Check query with operation name and tags")
	queryAndPrint(spanReader, tqp)

	tqp.DurationMin = 0
	tqp.DurationMax = time.Hour
	tqp.Tags = map[string]string{}
	logger.Info("check query with duration")
	queryAndPrint(spanReader, tqp)
}

func queryAndPrint(spanReader *cSpanStore.SpanReader, tqp *spanstore.TraceQueryParameters) {
	traces, err := spanReader.FindTraces(tqp)
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
	return &model.Span{
		TraceID:       model.TraceID{High: 1, Low: 2},
		SpanID:        model.SpanID(3),
		ParentSpanID:  model.SpanID(4),
		OperationName: "opName",
		References:    getReferences(),
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
			TraceID: model.TraceID{High: 1, Low: 1},
			SpanID:  model.SpanID(4),
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
