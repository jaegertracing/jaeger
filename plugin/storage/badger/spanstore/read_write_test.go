// Copyright (c) 2018 The Jaeger Authors.
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

package spanstore_test

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	bss "github.com/jaegertracing/jaeger/plugin/storage/badger/spanstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var _ io.Closer = new(bss.SpanWriter)

func TestWriteReadBack(t *testing.T) {
	runFactoryTest(t, func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
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
				err := sw.WriteSpan(&s)
				assert.NoError(t, err)
			}
		}

		for i := 0; i < traces; i++ {
			tr, err := sr.GetTrace(context.Background(), model.TraceID{
				Low:  uint64(i),
				High: 1,
			})
			assert.NoError(t, err)

			assert.Equal(t, spans, len(tr.Spans))
		}
	})
}

func TestValidation(t *testing.T) {
	runFactoryTest(t, func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
		tid := time.Now()
		params := &spanstore.TraceQueryParameters{
			StartTimeMin: tid,
			StartTimeMax: tid.Add(time.Duration(10)),
		}

		params.OperationName = "no-service"
		_, err := sr.FindTraces(context.Background(), params)
		assert.EqualError(t, err, "service name must be set")
		params.ServiceName = "find-service"

		_, err = sr.FindTraces(context.Background(), nil)
		assert.EqualError(t, err, "malformed request object")

		params.StartTimeMin = params.StartTimeMax.Add(1 * time.Hour)
		_, err = sr.FindTraces(context.Background(), params)
		assert.EqualError(t, err, "min start time is above max")
		params.StartTimeMin = tid

		params.DurationMax = time.Duration(1 * time.Millisecond)
		params.DurationMin = time.Duration(1 * time.Minute)
		_, err = sr.FindTraces(context.Background(), params)
		assert.EqualError(t, err, "min duration is above max")

		params = &spanstore.TraceQueryParameters{
			StartTimeMin: tid,
		}
		_, err = sr.FindTraces(context.Background(), params)
		assert.EqualError(t, err, "start and end time must be set")

		params.StartTimeMax = tid.Add(1 * time.Minute)
		params.Tags = map[string]string{"A": "B"}
		_, err = sr.FindTraces(context.Background(), params)
		assert.EqualError(t, err, "service name must be set")

		// Only StartTimeMin and Max (not supported yet)
		// _, err := sr.FindTraces(context.Background(), params)
		// assert.EqualError(t, err, "This query parameter is not supported yet")

	})
}

func TestIndexSeeks(t *testing.T) {
	runFactoryTest(t, func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
		startT := time.Now()
		traces := 60
		spans := 3
		tid := startT
		for i := 0; i < traces; i++ {
			tid = tid.Add(time.Duration(time.Millisecond * time.Duration(i)))

			for j := 0; j < spans; j++ {
				s := model.Span{
					TraceID: model.TraceID{
						Low:  uint64(i),
						High: 1,
					},
					SpanID:        model.SpanID(j),
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
				err := sw.WriteSpan(&s)
				assert.NoError(t, err)
			}
		}

		params := &spanstore.TraceQueryParameters{
			StartTimeMin: startT,
			StartTimeMax: startT.Add(time.Duration(time.Millisecond * 10)),
			ServiceName:  "service-1",
		}

		trs, err := sr.FindTraces(context.Background(), params)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(trs))

		params.OperationName = "operation-1"
		trs, err = sr.FindTraces(context.Background(), params)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(trs))

		params.ServiceName = "service-10" // this should not match
		trs, err = sr.FindTraces(context.Background(), params)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(trs))

		params.OperationName = "operation-4"
		trs, err = sr.FindTraces(context.Background(), params)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(trs))

		// Multi-index hits

		params.StartTimeMax = startT.Add(time.Duration(time.Millisecond * 666))
		params.ServiceName = "service-3"
		params.OperationName = "operation-1"
		tags := make(map[string]string)
		tags["k11"] = "val0"
		tags["error"] = "true"
		params.Tags = tags
		params.DurationMin = time.Duration(1 * time.Millisecond)
		// params.DurationMax = time.Duration(1 * time.Hour)
		trs, err = sr.FindTraces(context.Background(), params)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(trs))

		// Query limited amount of hits
		params.StartTimeMax = startT.Add(time.Duration(time.Hour * 1))
		delete(params.Tags, "k11")
		// delete(params.Tags, "error")
		params.NumTraces = 2
		trs, err = sr.FindTraces(context.Background(), params)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(trs))

		// Check for DESC return order
		params.NumTraces = 9
		trs, err = sr.FindTraces(context.Background(), params)
		assert.NoError(t, err)
		assert.Equal(t, 9, len(trs))

		// Assert that we fetched correctly in DESC time order
		for l := 1; l < len(trs); l++ {
			assert.True(t, trs[l].Spans[spans-1].StartTime.Before(trs[l-1].Spans[spans-1].StartTime))
		}

		// StartTime, endTime scan - full table scan (so technically no index seek)
		params = &spanstore.TraceQueryParameters{
			StartTimeMin: startT,
			StartTimeMax: startT.Add(time.Duration(time.Millisecond * 10)),
		}

		trs, err = sr.FindTraces(context.Background(), params)
		assert.NoError(t, err)
		assert.Equal(t, 6, len(trs))

		// StartTime and Duration queries
		params.StartTimeMax = startT.Add(time.Duration(time.Hour * 10))
		params.DurationMin = time.Duration(53 * time.Millisecond) // trace 51 (max)
		params.DurationMax = time.Duration(56 * time.Millisecond) // trace 56 (min)

		trs, err = sr.FindTraces(context.Background(), params)
		assert.NoError(t, err)
		assert.Equal(t, 6, len(trs))
	})
}

func TestFindNothing(t *testing.T) {
	runFactoryTest(t, func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
		startT := time.Now()
		params := &spanstore.TraceQueryParameters{
			StartTimeMin: startT,
			StartTimeMax: startT.Add(time.Duration(time.Millisecond * 10)),
			ServiceName:  "service-1",
		}

		trs, err := sr.FindTraces(context.Background(), params)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(trs))

		tr, err := sr.GetTrace(context.Background(), model.TraceID{High: 0, Low: 0})
		assert.NoError(t, err)
		assert.Nil(t, tr)
	})
}

func TestWriteDuplicates(t *testing.T) {
	runFactoryTest(t, func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
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
				err := sw.WriteSpan(&s)
				assert.NoError(t, err)
			}
		}
	})
}

func TestMenuSeeks(t *testing.T) {
	runFactoryTest(t, func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
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
				err := sw.WriteSpan(&s)
				assert.NoError(t, err)
			}
		}

		operations, err := sr.GetOperations(context.Background(), "service-1")
		assert.NoError(t, err)

		serviceList, err := sr.GetServices(context.Background())
		assert.NoError(t, err)

		assert.Equal(t, spans, len(operations))
		assert.Equal(t, services, len(serviceList))
	})
}

func TestPersist(t *testing.T) {
	dir, err := ioutil.TempDir("", "badgerTest")
	assert.NoError(t, err)

	p := func(t *testing.T, dir string, test func(t *testing.T, sw spanstore.Writer, sr spanstore.Reader)) {
		f := badger.NewFactory()
		opts := badger.NewOptions("badger")
		v, command := config.Viperize(opts.AddFlags)

		keyParam := fmt.Sprintf("--badger.directory-key=%s", dir)
		valueParam := fmt.Sprintf("--badger.directory-value=%s", dir)

		command.ParseFlags([]string{
			"--badger.ephemeral=false",
			"--badger.consistency=false",
			keyParam,
			valueParam,
		})
		f.InitFromViper(v)

		err = f.Initialize(metrics.NullFactory, zap.NewNop())
		assert.NoError(t, err)

		sw, err := f.CreateSpanWriter()
		assert.NoError(t, err)

		sr, err := f.CreateSpanReader()
		assert.NoError(t, err)

		defer func() {
			if closer, ok := sw.(io.Closer); ok {
				err := closer.Close()
				assert.NoError(t, err)
			} else {
				t.FailNow()
			}

		}()

		test(t, sw, sr)
	}

	p(t, dir, func(t *testing.T, sw spanstore.Writer, sr spanstore.Reader) {
		s := model.Span{
			TraceID: model.TraceID{
				Low:  uint64(1),
				High: 1,
			},
			SpanID:        model.SpanID(4),
			OperationName: fmt.Sprintf("operation-p"),
			Process: &model.Process{
				ServiceName: fmt.Sprintf("service-p"),
			},
			StartTime: time.Now(),
			Duration:  time.Duration(1 * time.Hour),
		}
		err := sw.WriteSpan(&s)
		assert.NoError(t, err)
	})

	p(t, dir, func(t *testing.T, sw spanstore.Writer, sr spanstore.Reader) {
		trace, err := sr.GetTrace(context.Background(), model.TraceID{
			Low:  uint64(1),
			High: 1,
		})
		assert.NoError(t, err)
		assert.Equal(t, "operation-p", trace.Spans[0].OperationName)

		services, err := sr.GetServices(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 1, len(services))
	})
}

// Opens a badger db and runs a a test on it.
func runFactoryTest(tb testing.TB, test func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader)) {
	f := badger.NewFactory()
	opts := badger.NewOptions("badger")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--badger.ephemeral=true",
		"--badger.consistency=false",
	})
	f.InitFromViper(v)

	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	assert.NoError(tb, err)

	sw, err := f.CreateSpanWriter()
	assert.NoError(tb, err)

	sr, err := f.CreateSpanReader()
	assert.NoError(tb, err)

	defer func() {
		if closer, ok := sw.(io.Closer); ok {
			err := closer.Close()
			assert.NoError(tb, err)
		} else {
			tb.FailNow()
		}

	}()
	test(tb, sw, sr)
}

func BenchmarkWrites(b *testing.B) {
	runFactoryTest(b, func(tb testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
		tid := time.Now()
		traces := 100000
		services := 4
		spans := 3
		b.StartTimer()
		b.SetParallelism(4)
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				for a := 0; a < b.N; a++ {
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
							_ = sw.WriteSpan(&s)
						}
					}
				}
			}
		})
		b.StopTimer()
	})
}
