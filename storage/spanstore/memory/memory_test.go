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

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"
)

var testingSpan = &model.Span{
	TraceID: model.TraceID{
		Low:  1,
		High: 2,
	},
	SpanID: model.SpanID(1),
	Process: &model.Process{
		ServiceName: "serviceName",
		Tags:        model.KeyValues{},
	},
	OperationName: "operationName",
	Tags: model.KeyValues{
		model.String("tagKey", "tagValue"),
	},
	Logs: []model.Log{
		{
			Timestamp: time.Now(),
			Fields: []model.KeyValue{
				model.String("logKey", "logValue"),
			},
		},
	},
	Duration:  time.Second * 5,
	StartTime: time.Unix(300, 0),
}

var childSpan1 = &model.Span{
	TraceID: model.TraceID{
		Low:  1,
		High: 2,
	},
	SpanID:       model.SpanID(2),
	ParentSpanID: model.SpanID(1),
	Process: &model.Process{
		ServiceName: "childService",
		Tags:        model.KeyValues{},
	},
	OperationName: "childOperationName",
	Tags: model.KeyValues{
		model.String("tagKey", "tagValue"),
	},
	Logs: []model.Log{
		{
			Timestamp: time.Now(),
			Fields: []model.KeyValue{
				model.String("logKey", "logValue"),
			},
		},
	},
	Duration:  time.Second * 5,
	StartTime: time.Unix(300, 0),
}

var childSpan2 = &model.Span{
	TraceID: model.TraceID{
		Low:  1,
		High: 2,
	},
	SpanID:       model.SpanID(3),
	ParentSpanID: model.SpanID(1),
	Process: &model.Process{
		ServiceName: "childService",
		Tags:        model.KeyValues{},
	},
	OperationName: "childOperationName",
	Tags: model.KeyValues{
		model.String("tagKey", "tagValue"),
	},
	Logs: []model.Log{
		{
			Timestamp: time.Now(),
			Fields: []model.KeyValue{
				model.String("logKey", "logValue"),
			},
		},
	},
	Duration:  time.Second * 5,
	StartTime: time.Unix(300, 0),
}

func withPopulatedMemoryStore(f func(store *Store)) {
	memStore := NewStore()
	memStore.WriteSpan(testingSpan)
	f(memStore)
}
func withMemoryStore(f func(store *Store)) {
	f(NewStore())
}

func TestStoreGetEmptyDependencies(t *testing.T) {
	withMemoryStore(func(store *Store) {
		links, err := store.GetDependencies(time.Now(), time.Hour)
		assert.NoError(t, err)
		assert.Empty(t, links)
	})
}

func TestStoreGetDependencies(t *testing.T) {
	withMemoryStore(func(store *Store) {
		assert.NoError(t, store.WriteSpan(testingSpan))
		assert.NoError(t, store.WriteSpan(childSpan1))
		assert.NoError(t, store.WriteSpan(childSpan2))
		links, err := store.GetDependencies(time.Now(), time.Hour)
		assert.NoError(t, err)
		assert.Empty(t, links)

		links, err = store.GetDependencies(time.Unix(0, 0).Add(time.Hour), time.Hour)
		assert.NoError(t, err)
		assert.Len(t, links, 1)
		assert.EqualValues(t, "serviceName", links[0].Parent)
		assert.EqualValues(t, "childService", links[0].Child)
		assert.EqualValues(t, 2, links[0].CallCount)
	})
}

func TestStoreWriteSpan(t *testing.T) {
	withMemoryStore(func(store *Store) {
		err := store.WriteSpan(testingSpan)
		assert.NoError(t, err)
	})
}

func TestStoreGetTraceSuccess(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		trace, err := store.GetTrace(testingSpan.TraceID)
		assert.NoError(t, err)
		assert.Len(t, trace.Spans, 1)
		assert.Equal(t, testingSpan, trace.Spans[0])
	})
}

func TestStoreGetTraceFailure(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		trace, err := store.GetTrace(model.TraceID{})
		assert.EqualError(t, err, errTraceNotFound.Error())
		assert.Nil(t, trace)
	})
}

func TestStoreGetServices(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		serviceNames, err := store.GetServices()
		assert.NoError(t, err)
		assert.Len(t, serviceNames, 1)
		assert.EqualValues(t, testingSpan.Process.ServiceName, serviceNames[0])
	})
}

func TestStoreGetOperationsFound(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		operations, err := store.GetOperations(testingSpan.Process.ServiceName)
		assert.NoError(t, err)
		assert.Len(t, operations, 1)
		assert.EqualValues(t, testingSpan.OperationName, operations[0])
	})
}

func TestStoreGetOperationsNotFound(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		operations, err := store.GetOperations("notAService")
		assert.NoError(t, err)
		assert.Len(t, operations, 0)
	})
}

func TestStoreGetEmptyTraceSet(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		traces, err := store.FindTraces(&spanstore.TraceQueryParameters{})
		assert.NoError(t, err)
		assert.Len(t, traces, 0)
	})
}

func TestStoreGetTrace(t *testing.T) {
	testStruct := []struct {
		query      *spanstore.TraceQueryParameters
		traceFound bool
	}{
		{
			&spanstore.TraceQueryParameters{
				ServiceName: testingSpan.Process.ServiceName,
			}, true,
		},
		{
			&spanstore.TraceQueryParameters{
				ServiceName: "wrongServiceName",
			}, false,
		},
		{
			&spanstore.TraceQueryParameters{
				ServiceName:   testingSpan.Process.ServiceName,
				OperationName: "wrongOperationName",
			}, false,
		},
		{
			&spanstore.TraceQueryParameters{
				ServiceName: testingSpan.Process.ServiceName,
				DurationMin: time.Second * 10,
			}, false,
		},
		{
			&spanstore.TraceQueryParameters{
				ServiceName: testingSpan.Process.ServiceName,
				DurationMax: time.Second * 2,
			}, false,
		},
		{
			&spanstore.TraceQueryParameters{
				ServiceName:  testingSpan.Process.ServiceName,
				StartTimeMin: time.Unix(500, 0),
			}, false,
		},
		{
			&spanstore.TraceQueryParameters{
				ServiceName:  testingSpan.Process.ServiceName,
				StartTimeMax: time.Unix(100, 0),
			}, false,
		},
		{
			&spanstore.TraceQueryParameters{
				ServiceName: testingSpan.Process.ServiceName,
				Tags: map[string]string{
					testingSpan.Tags[0].Key:           testingSpan.Tags[0].VStr,
					testingSpan.Logs[0].Fields[0].Key: testingSpan.Logs[0].Fields[0].VStr,
				},
			}, true,
		},
		{
			&spanstore.TraceQueryParameters{
				ServiceName: testingSpan.Process.ServiceName,
				Tags: map[string]string{
					testingSpan.Tags[0].Key: testingSpan.Logs[0].Fields[0].VStr,
				},
			}, false,
		},
	}
	for _, testS := range testStruct {
		withPopulatedMemoryStore(func(store *Store) {
			testS.query.NumTraces = 10
			traces, err := store.FindTraces(testS.query)
			assert.NoError(t, err)
			if testS.traceFound {
				assert.Len(t, traces, 1)
				assert.Len(t, traces[0].Spans, 1)
				assert.Equal(t, testingSpan, traces[0].Spans[0])
			} else {
				assert.Empty(t, traces)
			}
		})
	}
}
