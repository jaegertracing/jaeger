// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracegen

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Config describes the test scenario.
type Config struct {
	Workers         int
	Services        int
	Traces          int
	ChildSpans      int
	Attributes      int
	AttrKeys        int
	AttrValues      int
	Marshal         bool
	Debug           bool
	Firehose        bool
	Pause           time.Duration
	Duration        time.Duration
	Service         string
	TraceExporter   string
	Static          bool
	StaticTimeStart string
}

// Flags registers config flags.
func (c *Config) Flags(fs *flag.FlagSet) {
	fs.IntVar(&c.Workers, "workers", 1, "Number of workers (goroutines) to run")
	fs.IntVar(&c.Traces, "traces", 1, "Number of traces to generate in each worker (ignored if duration is provided)")
	fs.IntVar(&c.ChildSpans, "spans", 1, "Number of child spans to generate for each trace")
	fs.IntVar(&c.Attributes, "attrs", 11, "Number of attributes to generate for each child span")
	fs.IntVar(&c.AttrKeys, "attr-keys", 97, "Number of distinct attributes keys to use")
	fs.IntVar(&c.AttrValues, "attr-values", 1000, "Number of distinct values to allow for each attribute")
	fs.BoolVar(&c.Debug, "debug", false, "Whether to set DEBUG flag on the spans to force sampling")
	fs.BoolVar(&c.Firehose, "firehose", false, "Whether to set FIREHOSE flag on the spans to skip indexing")
	fs.DurationVar(&c.Pause, "pause", time.Microsecond, "How long to sleep before finishing each span. If set to 0s then a fake 123µs duration is used.")
	fs.DurationVar(&c.Duration, "duration", 0, "For how long to run the test if greater than 0s (overrides -traces).")
	fs.StringVar(&c.Service, "service", "tracegen", "Service name prefix to use")
	fs.IntVar(&c.Services, "services", 1, "Number of unique suffixes to add to service name when generating traces, e.g. tracegen-01 (but only one service per trace)")
	fs.StringVar(&c.TraceExporter, "trace-exporter", "otlp-http", "Trace exporter (otlp/otlp-http|otlp-grpc|stdout). Exporters can be additionally configured via environment variables, see https://github.com/jaegertracing/jaeger/blob/main/cmd/tracegen/README.md")
	fs.BoolVar(&c.Static, "static", false, "For generated static data to suit some Trace Bench, default: false")
	fs.StringVar(&c.StaticTimeStart, "static-timestart", "2025-01-01T00:00:00.000000000+00:00", "For support generated static data from this time point. default&eg. 2025-01-01T00:00:00.000000000+00:00")
}

// Run executes the test scenario.
func Run(c *Config, tracers []trace.Tracer, logger *zap.Logger) error {
	if c.Duration > 0 {
		c.Traces = 0
	} else if c.Traces <= 0 {
		return errors.New("either `traces` or `duration` must be greater than 0")
	}

	wg := sync.WaitGroup{}
	var running uint32 = 1
	if !c.Static {
		for i := 0; i < c.Workers; i++ {
			wg.Add(1)
			w := worker{
				id:      i,
				tracers: tracers,
				Config:  *c,
				running: &running,
				wg:      &wg,
				logger:  logger.With(zap.Int("worker", i)),
			}
			go w.simulateTraces()
		}
	} else {
		for i := 0; i < c.Workers; i++ {
			wg.Add(1)
			w := worker{
				id:      i,
				tracers: tracers,
				Config:  *c,
				running: &running,
				wg:      &wg,
				logger:  logger.With(zap.Int("worker", i)),
			}

			layout := "2006-01-02T15:04:05.000000000+00:00"
			t, err := time.Parse(layout, c.StaticTimeStart)
			if err != nil {
				panic("Static time start is not valid: " + err.Error() + "  check this flag format, default&eg.: 2025-01-01T00:00:00.000000000+00:00")
			}
			// mark
			startTime := t.Add(time.Microsecond)
			go w.simulateStaticTraces(startTime)
		}
	}

	if c.Duration > 0 {
		time.Sleep(c.Duration)
		atomic.StoreUint32(&running, 0)
	}
	wg.Wait()
	return nil
}

// PseudoRandomIDGenerator For static data generation
type PseudoRandomIDGenerator struct {
	mu  sync.Mutex
	rng *rand.Rand
}

// NewPseudoRandomIDGenerator  The Nth TracerProvider has a pseudo seed of N. / 应该改成可指定
func NewPseudoRandomIDGenerator(seed int) sdktrace.IDGenerator {
	src := rand.NewSource(int64(seed))
	return &PseudoRandomIDGenerator{
		rng: rand.New(src),
	}
}

func (g *PseudoRandomIDGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	g.mu.Lock()
	defer g.mu.Unlock()

	tid := trace.TraceID{}
	sid := trace.SpanID{}

	for {
		binary.NativeEndian.PutUint64(tid[:8], g.rng.Uint64())
		binary.NativeEndian.PutUint64(tid[8:], g.rng.Uint64())
		if tid.IsValid() {
			break
		}
	}

	for {
		binary.NativeEndian.PutUint64(sid[:], g.rng.Uint64())
		if sid.IsValid() {
			break
		}
	}

	return tid, sid
}

func (g *PseudoRandomIDGenerator) NewSpanID(ctx context.Context, traceID trace.TraceID) trace.SpanID {
	g.mu.Lock()
	defer g.mu.Unlock()

	sid := trace.SpanID{}
	for {
		binary.NativeEndian.PutUint64(sid[:], g.rng.Uint64())
		if sid.IsValid() {
			break
		}
	}
	return sid
}
