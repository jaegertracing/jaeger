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

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/memory/config"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	traceID     = model.NewTraceID(1, 2)
	testingSpan = makeTestingSpan(traceID, "")
)

var (
	traceID2     = model.NewTraceID(2, 3)
	testingSpan2 = makeTestingSpan(traceID2, "2")
)

var childSpan1 = &model.Span{
	TraceID:    traceID,
	SpanID:     model.NewSpanID(2),
	References: []model.SpanRef{model.NewChildOfRef(traceID, model.NewSpanID(1))},
	Process: &model.Process{
		ServiceName: "childService",
		Tags:        model.KeyValues{},
	},
	OperationName: "childOperationName",
	Tags: model.KeyValues{
		model.String("tagKey", "tagValue"),
		model.String("span.kind", "server"),
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
	TraceID:    traceID,
	SpanID:     model.NewSpanID(3),
	References: []model.SpanRef{model.NewChildOfRef(traceID, model.NewSpanID(1))},
	Process: &model.Process{
		ServiceName: "childService",
		Tags:        model.KeyValues{},
	},
	OperationName: "childOperationName",
	Tags: model.KeyValues{
		model.String("tagKey", "tagValue"),
		model.String("span.kind", "internal"),
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

var childSpan2_1 = &model.Span{
	TraceID: traceID,
	SpanID:  model.NewSpanID(4),
	// child of childSpan2, but with the same service name
	References: []model.SpanRef{model.NewChildOfRef(traceID, model.NewSpanID(3))},
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

// This kind of trace cannot be serialized
var nonSerializableSpan = &model.Span{
	Process: &model.Process{
		ServiceName: "naughtyService",
	},
	StartTime: time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
}

func withPopulatedMemoryStore(f func(store *Store)) {
	memStore := NewStore()
	memStore.WriteSpan(context.Background(), testingSpan)
	f(memStore)
}

func withMemoryStore(f func(store *Store)) {
	f(NewStore())
}

func TestStoreGetEmptyDependencies(t *testing.T) {
	// assert.Equal(t, testingSpan, testingSpan1B) // @@@
	withMemoryStore(func(store *Store) {
		links, err := store.GetDependencies(context.Background(), time.Now(), time.Hour)
		assert.NoError(t, err)
		assert.Empty(t, links)
	})
}

func TestStoreGetDependencies(t *testing.T) {
	withMemoryStore(func(store *Store) {
		assert.NoError(t, store.WriteSpan(context.Background(), testingSpan))
		assert.NoError(t, store.WriteSpan(context.Background(), childSpan1))
		assert.NoError(t, store.WriteSpan(context.Background(), childSpan2))
		assert.NoError(t, store.WriteSpan(context.Background(), childSpan2_1))
		links, err := store.GetDependencies(context.Background(), time.Now(), time.Hour)
		assert.NoError(t, err)
		assert.Empty(t, links)

		links, err = store.GetDependencies(context.Background(), time.Unix(0, 0).Add(time.Hour), time.Hour)
		assert.NoError(t, err)
		assert.Equal(t, []model.DependencyLink{{
			Parent:    "serviceName",
			Child:     "childService",
			CallCount: 2,
		}}, links)
	})
}

func TestStoreWriteSpan(t *testing.T) {
	withMemoryStore(func(store *Store) {
		err := store.WriteSpan(context.Background(), testingSpan)
		assert.NoError(t, err)
	})
}

func TestStoreWithLimit(t *testing.T) {
	maxTraces := 100
	store := WithConfiguration(config.Configuration{MaxTraces: maxTraces})

	for i := 0; i < maxTraces*2; i++ {
		id := model.NewTraceID(1, uint64(i))
		err := store.WriteSpan(context.Background(), &model.Span{
			TraceID: id,
			Process: &model.Process{
				ServiceName: "TestStoreWithLimit",
			},
		})
		assert.NoError(t, err)

		err = store.WriteSpan(context.Background(), &model.Span{
			TraceID: id,
			SpanID:  model.NewSpanID(uint64(i)),
			Process: &model.Process{
				ServiceName: "TestStoreWithLimit",
			},
			OperationName: "childOperationName",
		})
		assert.NoError(t, err)
	}

	assert.Equal(t, maxTraces, len(store.getTenant("").traces))
	assert.Equal(t, maxTraces, len(store.getTenant("").ids))
}

func TestStoreGetTraceSuccess(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		trace, err := store.GetTrace(context.Background(), testingSpan.TraceID)
		assert.NoError(t, err)
		assert.Len(t, trace.Spans, 1)
		assert.Equal(t, testingSpan, trace.Spans[0])
	})
}

func TestStoreGetAndMutateTrace(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		trace, err := store.GetTrace(context.Background(), testingSpan.TraceID)
		assert.NoError(t, err)
		assert.Len(t, trace.Spans, 1)
		assert.Equal(t, testingSpan, trace.Spans[0])
		assert.Len(t, trace.Spans[0].Warnings, 0)

		trace.Spans[0].Warnings = append(trace.Spans[0].Warnings, "the end is near")

		trace, err = store.GetTrace(context.Background(), testingSpan.TraceID)
		assert.NoError(t, err)
		assert.Len(t, trace.Spans, 1)
		assert.Equal(t, testingSpan, trace.Spans[0])
		assert.Len(t, trace.Spans[0].Warnings, 0)
	})
}

func TestStoreGetTraceError(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		store.getTenant("").traces[testingSpan.TraceID] = &model.Trace{
			Spans: []*model.Span{nonSerializableSpan},
		}
		_, err := store.GetTrace(context.Background(), testingSpan.TraceID)
		assert.Error(t, err)
	})
}

func TestStoreGetTraceFailure(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		trace, err := store.GetTrace(context.Background(), model.TraceID{})
		assert.EqualError(t, err, spanstore.ErrTraceNotFound.Error())
		assert.Nil(t, trace)
	})
}

func TestStoreGetServices(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		serviceNames, err := store.GetServices(context.Background())
		assert.NoError(t, err)
		assert.Len(t, serviceNames, 1)
		assert.EqualValues(t, testingSpan.Process.ServiceName, serviceNames[0])
	})
}

func TestStoreGetAllOperationsFound(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		assert.NoError(t, store.WriteSpan(context.Background(), testingSpan))
		assert.NoError(t, store.WriteSpan(context.Background(), childSpan1))
		assert.NoError(t, store.WriteSpan(context.Background(), childSpan2))
		assert.NoError(t, store.WriteSpan(context.Background(), childSpan2_1))
		operations, err := store.GetOperations(
			context.Background(),
			spanstore.OperationQueryParameters{ServiceName: childSpan1.Process.ServiceName},
		)
		assert.NoError(t, err)
		assert.Len(t, operations, 3)
		assert.EqualValues(t, childSpan1.OperationName, operations[0].Name)
	})
}

func TestStoreGetServerOperationsFound(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		assert.NoError(t, store.WriteSpan(context.Background(), testingSpan))
		assert.NoError(t, store.WriteSpan(context.Background(), childSpan1))
		assert.NoError(t, store.WriteSpan(context.Background(), childSpan2))
		assert.NoError(t, store.WriteSpan(context.Background(), childSpan2_1))
		expected := []spanstore.Operation{
			{Name: childSpan1.OperationName, SpanKind: "server"},
		}
		operations, err := store.GetOperations(context.Background(),
			spanstore.OperationQueryParameters{
				ServiceName: childSpan1.Process.ServiceName,
				SpanKind:    "server",
			})
		assert.NoError(t, err)
		assert.Len(t, operations, 1)
		assert.Equal(t, expected, operations)
	})
}

func TestStoreGetOperationsNotFound(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		operations, err := store.GetOperations(
			context.Background(),
			spanstore.OperationQueryParameters{ServiceName: "notAService"},
		)
		assert.NoError(t, err)
		assert.Len(t, operations, 0)
	})
}

func TestStoreGetEmptyTraceSet(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		traces, err := store.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
		assert.NoError(t, err)
		assert.Len(t, traces, 0)
	})
}

func TestStoreFindTracesError(t *testing.T) {
	withPopulatedMemoryStore(func(store *Store) {
		err := store.WriteSpan(context.Background(), nonSerializableSpan)
		assert.NoError(t, err)
		_, err = store.FindTraces(context.Background(), &spanstore.TraceQueryParameters{ServiceName: "naughtyService"})
		assert.Error(t, err)
	})
}

func TestStoreFindTracesLimitGetsMostRecent(t *testing.T) {
	storeSize, querySize := 100, 10

	// This slice is in order from oldest to newest trace.
	// Store keeps spans in a map, so storage order is effectively random.
	// This ensures that query results include the most recent traces when limit < results.

	var spans []*model.Span
	for i := 0; i < storeSize; i++ {
		spans = append(spans,
			&model.Span{
				TraceID:       model.NewTraceID(1, uint64(i)),
				SpanID:        model.NewSpanID(1),
				OperationName: "operationName",
				Duration:      time.Second,
				StartTime:     time.Unix(int64(i*24*60*60), 0),
				Process: &model.Process{
					ServiceName: "serviceName",
				},
			})
	}

	// Want the two most recent spans, not any two spans
	var expectedTraces []*model.Trace
	for _, span := range spans[storeSize-querySize:] {
		trace := &model.Trace{
			Spans: []*model.Span{span},
		}
		expectedTraces = append(expectedTraces, trace)
	}

	memStore := NewStore()
	for _, span := range spans {
		memStore.WriteSpan(context.Background(), span)
	}

	gotTraces, err := memStore.FindTraces(context.Background(), &spanstore.TraceQueryParameters{
		ServiceName: "serviceName",
		NumTraces:   querySize,
	})

	assert.NoError(t, err)
	if assert.Len(t, gotTraces, len(expectedTraces)) {
		for i := range gotTraces {
			assert.EqualValues(t, expectedTraces[i].Spans[0].StartTime.Unix(), gotTraces[i].Spans[0].StartTime.Unix())
		}
	}
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
			traces, err := store.FindTraces(context.Background(), testS.query)
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

func TestStore_FindTraceIDs(t *testing.T) {
	withMemoryStore(func(store *Store) {
		traceIDs, err := store.FindTraceIDs(context.Background(), nil)
		assert.Nil(t, traceIDs)
		assert.EqualError(t, err, "not implemented")
	})
}

func TestTenantStore(t *testing.T) {
	withMemoryStore(func(store *Store) {
		ctxAcme := tenancy.WithTenant(context.Background(), "acme")
		ctxWonka := tenancy.WithTenant(context.Background(), "wonka")

		assert.NoError(t, store.WriteSpan(ctxAcme, testingSpan))
		assert.NoError(t, store.WriteSpan(ctxWonka, testingSpan2))

		// Can we retrieve the spans with correct tenancy
		trace1, err := store.GetTrace(ctxAcme, testingSpan.TraceID)
		assert.NoError(t, err)
		assert.Len(t, trace1.Spans, 1)
		assert.Equal(t, testingSpan, trace1.Spans[0])

		trace2, err := store.GetTrace(ctxWonka, testingSpan2.TraceID)
		assert.NoError(t, err)
		assert.Len(t, trace2.Spans, 1)
		assert.Equal(t, testingSpan2, trace2.Spans[0])

		// Can we query the spans with correct tenancy
		traces1, err := store.FindTraces(ctxAcme, &spanstore.TraceQueryParameters{
			ServiceName: "serviceName",
		})
		assert.NoError(t, err)
		assert.Len(t, traces1, 1)
		assert.Len(t, traces1[0].Spans, 1)
		assert.Equal(t, testingSpan, traces1[0].Spans[0])

		traces2, err := store.FindTraces(ctxWonka, &spanstore.TraceQueryParameters{
			ServiceName: "serviceName2",
		})
		assert.NoError(t, err)
		assert.Len(t, traces2, 1)
		assert.Len(t, traces2[0].Spans, 1)
		assert.Equal(t, testingSpan2, traces2[0].Spans[0])

		// Do the spans fail with incorrect tenancy?
		_, err = store.GetTrace(ctxAcme, testingSpan2.TraceID)
		assert.Error(t, err)

		_, err = store.GetTrace(ctxWonka, testingSpan.TraceID)
		assert.Error(t, err)

		_, err = store.GetTrace(context.Background(), testingSpan.TraceID)
		assert.Error(t, err)
	})
}

func makeTestingSpan(traceID model.TraceID, suffix string) *model.Span {
	return &model.Span{
		TraceID: traceID,
		SpanID:  model.NewSpanID(1),
		Process: &model.Process{
			ServiceName: "serviceName" + suffix,
			Tags:        []model.KeyValue(nil),
		},
		OperationName: "operationName" + suffix,
		Tags: model.KeyValues{
			model.String("tagKey", "tagValue"+suffix),
			model.String("span.kind", "client"+suffix),
		},
		Logs: []model.Log{
			{
				Timestamp: time.Now().UTC(),
				Fields: []model.KeyValue{
					model.String("logKey", "logValue"+suffix),
				},
			},
		},
		Duration:  time.Second * 5,
		StartTime: time.Unix(300, 0).UTC(),
	}
}
