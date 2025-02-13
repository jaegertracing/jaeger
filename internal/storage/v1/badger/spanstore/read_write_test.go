// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore_test

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

func TestWriteReadBack(t *testing.T) {
	runFactoryTest(t, func(_ testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
		tid := time.Now()
		traces := 40
		spans := 3

		dummyKv := []model.KeyValue{
			{
				Key:   "key",
				VType: model.StringType,
				VStr:  "value",
			},
		}

		for i := 0; i < traces; i++ {
			for j := 0; j < spans; j++ {
				s := model.Span{
					TraceID: model.TraceID{
						Low:  uint64(i),
						High: 1,
					},
					SpanID:        model.SpanID(j),
					OperationName: "operation",
					Process: &model.Process{
						ServiceName: "service",
						Tags:        dummyKv,
					},
					StartTime: tid.Add(time.Duration(i)),
					Duration:  time.Duration(i + j),
					Tags:      dummyKv,
					Logs: []model.Log{
						{
							Timestamp: tid,
							Fields:    dummyKv,
						},
					},
				}
				err := sw.WriteSpan(context.Background(), &s)
				require.NoError(t, err)
			}
		}

		for i := 0; i < traces; i++ {
			tr, err := sr.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: model.TraceID{
				Low:  uint64(i),
				High: 1,
			}})
			require.NoError(t, err)

			assert.Len(t, tr.Spans, spans)
		}
	})
}

func TestValidation(t *testing.T) {
	runFactoryTest(t, func(_ testing.TB, _ spanstore.Writer, sr spanstore.Reader) {
		tid := time.Now()
		params := &spanstore.TraceQueryParameters{
			StartTimeMin: tid,
			StartTimeMax: tid.Add(time.Duration(10)),
		}

		params.OperationName = "no-service"
		_, err := sr.FindTraces(context.Background(), params)
		require.EqualError(t, err, "service name must be set")
		params.ServiceName = "find-service"

		_, err = sr.FindTraces(context.Background(), nil)
		require.EqualError(t, err, "malformed request object")

		params.StartTimeMin = params.StartTimeMax.Add(1 * time.Hour)
		_, err = sr.FindTraces(context.Background(), params)
		require.EqualError(t, err, "min start time is above max")
		params.StartTimeMin = tid

		params.DurationMax = time.Duration(1 * time.Millisecond)
		params.DurationMin = time.Duration(1 * time.Minute)
		_, err = sr.FindTraces(context.Background(), params)
		require.EqualError(t, err, "min duration is above max")

		params = &spanstore.TraceQueryParameters{
			StartTimeMin: tid,
		}
		_, err = sr.FindTraces(context.Background(), params)
		require.EqualError(t, err, "start and end time must be set")

		params.StartTimeMax = tid.Add(1 * time.Minute)
		params.Tags = map[string]string{"A": "B"}
		_, err = sr.FindTraces(context.Background(), params)
		require.EqualError(t, err, "service name must be set")
	})
}

func TestIndexSeeks(t *testing.T) {
	runFactoryTest(t, func(_ testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
		startT := time.Now()
		traces := 60
		spans := 3
		tid := startT

		traceOrder := make([]uint64, traces)

		for i := 0; i < traces; i++ {
			lowId := rand.Uint64()
			traceOrder[i] = lowId
			tid = tid.Add(time.Duration(time.Millisecond * time.Duration(i)))

			for j := 0; j < spans; j++ {
				s := model.Span{
					TraceID: model.TraceID{
						Low:  lowId,
						High: 1,
					},
					SpanID:        model.SpanID(rand.Uint64()),
					OperationName: fmt.Sprintf("operation-%d", j),
					Process: &model.Process{
						ServiceName: fmt.Sprintf("service-%d", i%4),
					},
					StartTime: tid,
					Duration:  time.Duration(time.Duration(i+j) * time.Millisecond),
					Tags: model.KeyValues{
						model.KeyValue{
							Key:   fmt.Sprintf("k%d", i),
							VStr:  fmt.Sprintf("val%d", j),
							VType: model.StringType,
						},
						{
							Key:   "error",
							VType: model.BoolType,
							VBool: true,
						},
					},
				}

				err := sw.WriteSpan(context.Background(), &s)
				require.NoError(t, err)
			}
		}

		testOrder := func(trs []*model.Trace) {
			// Assert that we returned correctly in DESC time order
			for l := 1; l < len(trs); l++ {
				assert.True(t, trs[l].Spans[spans-1].StartTime.Before(trs[l-1].Spans[spans-1].StartTime))
			}
		}

		params := &spanstore.TraceQueryParameters{
			StartTimeMin: startT,
			StartTimeMax: startT.Add(time.Duration(time.Millisecond * 10)),
			ServiceName:  "service-1",
		}

		trs, err := sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, trs, 1)
		assert.Len(t, trs[0].Spans, spans)

		params.OperationName = "operation-1"
		trs, err = sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, trs, 1)

		params.ServiceName = "service-10" // this should not match
		trs, err = sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Empty(t, trs)

		params.OperationName = "operation-4"
		trs, err = sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Empty(t, trs)

		// Multi-index hits

		params.StartTimeMax = startT.Add(time.Duration(time.Millisecond * 666))
		params.ServiceName = "service-3"
		params.OperationName = "operation-1"
		tags := make(map[string]string)
		tags["k11"] = "val0"
		tags["error"] = "true"
		params.Tags = tags
		params.DurationMin = time.Duration(1 * time.Millisecond)
		trs, err = sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, trs, 1)
		assert.Len(t, trs[0].Spans, spans)

		// Query limited amount of hits

		params.StartTimeMax = startT.Add(time.Duration(time.Hour * 1))
		delete(params.Tags, "k11")
		params.NumTraces = 2
		trs, err = sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, trs, 2)
		assert.Equal(t, traceOrder[59], trs[0].Spans[0].TraceID.Low)
		assert.Equal(t, traceOrder[55], trs[1].Spans[0].TraceID.Low)
		testOrder(trs)

		// Check for DESC return order with duration index
		params = &spanstore.TraceQueryParameters{
			StartTimeMin: startT,
			StartTimeMax: startT.Add(time.Duration(time.Hour * 1)),
			DurationMin:  time.Duration(30 * time.Millisecond), // Filters one
			DurationMax:  time.Duration(50 * time.Millisecond), // Filters three
			NumTraces:    9,
		}
		trs, err = sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, trs, 9) // Returns 23, we limited to 9

		// Check the newest items are returned
		assert.Equal(t, traceOrder[50], trs[0].Spans[0].TraceID.Low)
		assert.Equal(t, traceOrder[42], trs[8].Spans[0].TraceID.Low)
		testOrder(trs)

		// Check for DESC return order without duration index, but still with limit
		params.DurationMin = 0
		params.DurationMax = 0
		params.NumTraces = 7
		trs, err = sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, trs, 7)
		assert.Equal(t, traceOrder[59], trs[0].Spans[0].TraceID.Low)
		assert.Equal(t, traceOrder[53], trs[6].Spans[0].TraceID.Low)
		testOrder(trs)

		// StartTime, endTime scan - full table scan (so technically no index seek)
		params = &spanstore.TraceQueryParameters{
			StartTimeMin: startT,
			StartTimeMax: startT.Add(time.Duration(time.Millisecond * 10)),
		}

		trs, err = sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, trs, 5)
		assert.Len(t, trs[0].Spans, spans)
		testOrder(trs)

		// StartTime and Duration queries
		params.StartTimeMax = startT.Add(time.Duration(time.Hour * 10))
		params.DurationMin = time.Duration(53 * time.Millisecond) // trace 51 (min)
		params.DurationMax = time.Duration(56 * time.Millisecond) // trace 56 (max)

		trs, err = sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, trs, 6)
		assert.Equal(t, traceOrder[56], trs[0].Spans[0].TraceID.Low)
		assert.Equal(t, traceOrder[51], trs[5].Spans[0].TraceID.Low)
		testOrder(trs)
	})
}

func TestFindNothing(t *testing.T) {
	runFactoryTest(t, func(_ testing.TB, _ spanstore.Writer, sr spanstore.Reader) {
		startT := time.Now()
		params := &spanstore.TraceQueryParameters{
			StartTimeMin: startT,
			StartTimeMax: startT.Add(time.Duration(time.Millisecond * 10)),
			ServiceName:  "service-1",
		}

		trs, err := sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Empty(t, trs)

		tr, err := sr.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: model.TraceID{Low: 0, High: 0}})
		assert.Equal(t, spanstore.ErrTraceNotFound, err)
		assert.Nil(t, tr)
	})
}

func TestWriteDuplicates(t *testing.T) {
	runFactoryTest(t, func(_ testing.TB, sw spanstore.Writer, _ spanstore.Reader) {
		tid := time.Now()
		times := 40
		spans := 3
		for i := 0; i < times; i++ {
			for j := 0; j < spans; j++ {
				s := model.Span{
					TraceID: model.TraceID{
						Low:  uint64(0),
						High: 1,
					},
					SpanID:        model.SpanID(j),
					OperationName: "operation",
					Process: &model.Process{
						ServiceName: "service",
					},
					StartTime: tid.Add(time.Duration(10)),
					Duration:  time.Duration(i + j),
				}
				err := sw.WriteSpan(context.Background(), &s)
				require.NoError(t, err)
			}
		}
	})
}

func TestMenuSeeks(t *testing.T) {
	runFactoryTest(t, func(_ testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
		tid := time.Now()
		traces := 40
		services := 4
		spans := 3
		for i := 0; i < traces; i++ {
			for j := 0; j < spans; j++ {
				s := model.Span{
					TraceID: model.TraceID{
						Low:  uint64(i),
						High: 1,
					},
					SpanID:        model.SpanID(j),
					OperationName: fmt.Sprintf("operation-%d", j),
					Process: &model.Process{
						ServiceName: fmt.Sprintf("service-%d", i%services),
					},
					StartTime: tid.Add(time.Duration(i)),
					Duration:  time.Duration(i + j),
				}
				err := sw.WriteSpan(context.Background(), &s)
				require.NoError(t, err)
			}
		}

		operations, err := sr.GetOperations(
			context.Background(),
			spanstore.OperationQueryParameters{ServiceName: "service-1"},
		)
		require.NoError(t, err)

		serviceList, err := sr.GetServices(context.Background())
		require.NoError(t, err)

		assert.Len(t, operations, spans)
		assert.Len(t, serviceList, services)
	})
}

func TestPersist(t *testing.T) {
	dir := t.TempDir()

	p := func(t *testing.T, dir string, test func(t *testing.T, sw spanstore.Writer, sr spanstore.Reader)) {
		f := badger.NewFactory()
		defer func() {
			require.NoError(t, f.Close())
		}()

		cfg := badger.DefaultConfig()
		v, command := config.Viperize(cfg.AddFlags)

		keyParam := "--badger.directory-key=" + dir
		valueParam := "--badger.directory-value=" + dir

		command.ParseFlags([]string{
			"--badger.ephemeral=false",
			"--badger.consistency=false",
			keyParam,
			valueParam,
		})
		f.InitFromViper(v, zap.NewNop())

		err := f.Initialize(metrics.NullFactory, zap.NewNop())
		require.NoError(t, err)

		sw, err := f.CreateSpanWriter()
		require.NoError(t, err)

		sr, err := f.CreateSpanReader()
		require.NoError(t, err)

		test(t, sw, sr)
	}

	p(t, dir, func(t *testing.T, sw spanstore.Writer, _ spanstore.Reader) {
		s := model.Span{
			TraceID: model.TraceID{
				Low:  uint64(1),
				High: 1,
			},
			SpanID:        model.SpanID(4),
			OperationName: "operation-p",
			Process: &model.Process{
				ServiceName: "service-p",
			},
			StartTime: time.Now(),
			Duration:  time.Duration(1 * time.Hour),
		}
		err := sw.WriteSpan(context.Background(), &s)
		require.NoError(t, err)
	})

	p(t, dir, func(t *testing.T, _ spanstore.Writer, sr spanstore.Reader) {
		trace, err := sr.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: model.TraceID{
			Low:  uint64(1),
			High: 1,
		}})
		require.NoError(t, err)
		assert.Equal(t, "operation-p", trace.Spans[0].OperationName)

		services, err := sr.GetServices(context.Background())
		require.NoError(t, err)
		assert.Len(t, services, 1)
	})
}

// Opens a badger db and runs a test on it.
func runFactoryTest(tb testing.TB, test func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader)) {
	f := badger.NewFactory()
	defer func() {
		require.NoError(tb, f.Close())
	}()

	cfg := badger.DefaultConfig()
	v, command := config.Viperize(cfg.AddFlags)
	command.ParseFlags([]string{
		"--badger.ephemeral=true",
		"--badger.consistency=false",
	})
	f.InitFromViper(v, zap.NewNop())

	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(tb, err)

	sw, err := f.CreateSpanWriter()
	require.NoError(tb, err)

	sr, err := f.CreateSpanReader()
	require.NoError(tb, err)

	test(tb, sw, sr)
}

// Benchmarks intended for profiling

func writeSpans(sw spanstore.Writer, tags []model.KeyValue, services, operations []string, traces, spans int, high uint64, tid time.Time) {
	for i := 0; i < traces; i++ {
		for j := 0; j < spans; j++ {
			s := model.Span{
				TraceID: model.TraceID{
					Low:  uint64(i),
					High: high,
				},
				SpanID:        model.SpanID(j),
				OperationName: operations[j],
				Process: &model.Process{
					ServiceName: services[j],
				},
				Tags:      tags,
				StartTime: tid.Add(time.Duration(time.Millisecond)),
				Duration:  time.Duration(time.Millisecond * time.Duration(i+j)),
			}
			_ = sw.WriteSpan(context.Background(), &s)
		}
	}
}

func BenchmarkWrites(b *testing.B) {
	runFactoryTest(b, func(_ testing.TB, sw spanstore.Writer, _ spanstore.Reader) {
		tid := time.Now()
		traces := 1000
		spans := 32
		tagsCount := 64
		tags, services, operations := makeWriteSupports(tagsCount, spans)

		f, err := os.Create("writes.out")
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()

		b.ResetTimer()
		for a := 0; a < b.N; a++ {
			writeSpans(sw, tags, services, operations, traces, spans, uint64(0), tid)
		}
		b.StopTimer()
	})
}

func makeWriteSupports(tagsCount, spans int) (tags []model.KeyValue, services []string, operations []string) {
	tags = make([]model.KeyValue, tagsCount)
	for i := 0; i < tagsCount; i++ {
		tags[i] = model.KeyValue{
			Key:  fmt.Sprintf("a%d", i),
			VStr: fmt.Sprintf("b%d", i),
		}
	}
	operations = make([]string, spans)
	for j := 0; j < spans; j++ {
		operations[j] = fmt.Sprintf("operation-%d", j)
	}
	services = make([]string, spans)
	for i := 0; i < spans; i++ {
		services[i] = fmt.Sprintf("service-%d", i)
	}

	return tags, services, operations
}

func makeReadBenchmark(b *testing.B, _ time.Time, params *spanstore.TraceQueryParameters, outputFile string) {
	runLargeFactoryTest(b, func(_ testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
		tid := time.Now()

		// Total amount of traces is traces * tracesTimes
		traces := 1000
		tracesTimes := 1

		// Total amount of spans written is traces * tracesTimes * spans
		spans := 32

		// Default is 160k

		tagsCount := 64
		tags, services, operations := makeWriteSupports(tagsCount, spans)

		for h := 0; h < tracesTimes; h++ {
			writeSpans(sw, tags, services, operations, traces, spans, uint64(h), tid)
		}

		f, err := os.Create(outputFile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()

		b.ResetTimer()
		for a := 0; a < b.N; a++ {
			sr.FindTraces(context.Background(), params)
		}
		b.StopTimer()
	})
}

func BenchmarkServiceTagsRangeQueryLimitIndexFetch(b *testing.B) {
	tid := time.Now()
	params := &spanstore.TraceQueryParameters{
		StartTimeMin: tid,
		StartTimeMax: tid.Add(time.Duration(time.Millisecond * 2000)),
		ServiceName:  "service-1",
		Tags: map[string]string{
			"a8": "b8",
		},
	}

	params.DurationMin = time.Duration(1 * time.Millisecond) // durationQuery takes 53% of total execution time..
	params.NumTraces = 50

	makeReadBenchmark(b, tid, params, "scanrangeandindexlimit.out")
}

func BenchmarkServiceIndexLimitFetch(b *testing.B) {
	tid := time.Now()
	params := &spanstore.TraceQueryParameters{
		StartTimeMin: tid,
		StartTimeMax: tid.Add(time.Duration(time.Millisecond * 2000)),
		ServiceName:  "service-1",
	}

	params.NumTraces = 50

	makeReadBenchmark(b, tid, params, "serviceindexlimit.out")
}

// Opens a badger db and runs a test on it.
func runLargeFactoryTest(tb testing.TB, test func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader)) {
	assertion := require.New(tb)
	f := badger.NewFactory()
	cfg := badger.DefaultConfig()
	v, command := config.Viperize(cfg.AddFlags)

	dir := filepath.Join(tb.TempDir(), "badger-testRun")
	err := os.MkdirAll(dir, 0o700)
	assertion.NoError(err)
	keyParam := "--badger.directory-key=" + dir
	valueParam := "--badger.directory-value=" + dir

	command.ParseFlags([]string{
		"--badger.ephemeral=false",
		"--badger.consistency=false", // Consistency is false as default to reduce effect of disk speed
		keyParam,
		valueParam,
	})

	f.InitFromViper(v, zap.NewNop())

	err = f.Initialize(metrics.NullFactory, zap.NewNop())
	assertion.NoError(err)

	sw, err := f.CreateSpanWriter()
	assertion.NoError(err)

	sr, err := f.CreateSpanReader()
	assertion.NoError(err)

	defer func() {
		err := f.Close()
		os.RemoveAll(dir)
		require.NoError(tb, err)
	}()
	test(tb, sw, sr)
}

// TestRandomTraceID from issue #1808
func TestRandomTraceID(t *testing.T) {
	runFactoryTest(t, func(_ testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
		s1 := model.Span{
			TraceID: model.TraceID{
				Low:  uint64(14767110704788176287),
				High: 0,
			},
			SpanID:        model.SpanID(14976775253976086374),
			OperationName: "/",
			Process: &model.Process{
				ServiceName: "nginx",
			},
			Tags: model.KeyValues{
				model.KeyValue{
					Key:   "http.request_id",
					VStr:  "first",
					VType: model.StringType,
				},
			},
			StartTime: time.Now(),
			Duration:  1 * time.Second,
		}
		err := sw.WriteSpan(context.Background(), &s1)
		require.NoError(t, err)

		s2 := model.Span{
			TraceID: model.TraceID{
				Low:  uint64(4775132888371984950),
				High: 0,
			},
			SpanID:        model.SpanID(13576481569227028654),
			OperationName: "/",
			Process: &model.Process{
				ServiceName: "nginx",
			},
			Tags: model.KeyValues{
				model.KeyValue{
					Key:   "http.request_id",
					VStr:  "second",
					VType: model.StringType,
				},
			},
			StartTime: time.Now(),
			Duration:  1 * time.Second,
		}
		err = sw.WriteSpan(context.Background(), &s2)
		require.NoError(t, err)

		params := &spanstore.TraceQueryParameters{
			StartTimeMin: time.Now().Add(-1 * time.Minute),
			StartTimeMax: time.Now(),
			ServiceName:  "nginx",
			Tags: map[string]string{
				"http.request_id": "second",
			},
		}
		traces, err := sr.FindTraces(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, traces, 1)
	})
}
