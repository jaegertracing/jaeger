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

package processor

import (
	"io"
	"time"

	"github.com/uber/jaeger-lib/metrics"
)

type metricsDecorator struct {
	errors    metrics.Counter
	latency   metrics.Timer
	processor SpanProcessor
	io.Closer
}

// NewDecoratedProcessor returns a processor with metrics
func NewDecoratedProcessor(f metrics.Factory, processor SpanProcessor) SpanProcessor {
	m := f.Namespace(metrics.NSOptions{Name: "span-processor", Tags: nil})
	return &metricsDecorator{
		errors:    m.Counter(metrics.Options{Name: "errors", Tags: nil}),
		latency:   m.Timer(metrics.TimerOptions{Name: "latency", Tags: nil}),
		processor: processor,
	}
}

func (d *metricsDecorator) Process(message Message) error {
	now := time.Now()

	err := d.processor.Process(message)
	d.latency.Record(time.Since(now))
	if err != nil {
		d.errors.Inc(1)
	}
	return err
}
