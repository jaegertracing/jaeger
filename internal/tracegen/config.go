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

package tracegen

import (
	"flag"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Config describes the test scenario.
type Config struct {
	Workers       int
	Services      int
	Traces        int
	ChildSpans    int
	Attributes    int
	AttrKeys      int
	AttrValues    int
	Marshal       bool
	Debug         bool
	Firehose      bool
	Pause         time.Duration
	Duration      time.Duration
	Service       string
	TraceExporter string
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
	fs.StringVar(&c.TraceExporter, "trace-exporter", "jaeger", "Trace exporter (jaeger|otlp/otlp-http|otlp-grpc|stdout). Exporters can be additionally configured via environment variables, see https://github.com/jaegertracing/jaeger/blob/main/cmd/tracegen/README.md")
}

// Run executes the test scenario.
func Run(c *Config, tracers []trace.Tracer, logger *zap.Logger) error {
	if c.Duration > 0 {
		c.Traces = 0
	} else if c.Traces <= 0 {
		return fmt.Errorf("either `traces` or `duration` must be greater than 0")
	}

	wg := sync.WaitGroup{}
	var running uint32 = 1
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
	if c.Duration > 0 {
		time.Sleep(c.Duration)
		atomic.StoreUint32(&running, 0)
	}
	wg.Wait()
	return nil
}
