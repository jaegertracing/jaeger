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
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type worker struct {
	tracer   trace.Tracer
	running  *uint32         // pointer to shared flag that indicates it's time to stop the test
	id       int             // worker id
	traces   int             // how many traces the worker has to generate (only when duration==0)
	marshal  bool            // whether the worker needs to marshal trace context via HTTP headers
	debug    bool            // whether to set DEBUG flag on the spans
	firehose bool            // whether to set FIREHOSE flag on the spans
	duration time.Duration   // how long to run the test for (overrides `traces`)
	pause    time.Duration   // how long to pause before finishing the trace
	wg       *sync.WaitGroup // notify when done
	logger   *zap.Logger
}

const (
	fakeSpanDuration = 123 * time.Microsecond
)

func (w worker) simulateTraces() {
	var i int
	for atomic.LoadUint32(w.running) == 1 {
		w.simulateOneTrace()
		i++
		if w.traces != 0 {
			if i >= w.traces {
				break
			}
		}
	}
	w.logger.Info(fmt.Sprintf("Worker %d generated %d traces", w.id, i))
	w.wg.Done()
}

func (w worker) simulateOneTrace() {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("peer.service", "tracegen-server"),
		attribute.String("peer.host.ipv4", "1.1.1.1"),
	}
	if w.debug {
		attrs = append(attrs, attribute.Bool("jaeger.debug", true))
	}
	if w.firehose {
		attrs = append(attrs, attribute.Bool("jaeger.firehose", true))
	}
	start := time.Now()
	ctx, sp := w.tracer.Start(
		ctx,
		"lets-go",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
		trace.WithTimestamp(start),
	)

	_, child := w.tracer.Start(
		ctx,
		"okey-dokey",
		trace.WithSpanKind(trace.SpanKindServer),
	)

	time.Sleep(w.pause)

	if w.pause != 0 {
		child.End()
		sp.End()
	} else {
		child.End(
			trace.WithTimestamp(start.Add(fakeSpanDuration)),
		)
		sp.End(
			trace.WithTimestamp(start.Add(fakeSpanDuration)),
		)
	}
}
