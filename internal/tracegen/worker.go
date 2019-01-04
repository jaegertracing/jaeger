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
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"go.uber.org/zap"
)

type worker struct {
	running  *uint32         // pointer to shared flag that indicates it's time to stop the test
	id       int             // worker id
	traces   int             // how many traces the worker has to generate (only when duration==0)
	marshal  bool            // whether the worker needs to marshal trace context via HTTP headers
	debug    bool            // whether to set DEBUG flag on the spans
	duration time.Duration   // how long to run the test for (overrides `traces`)
	pause    time.Duration   // how long to pause before finishing the trace
	wg       *sync.WaitGroup // notify when done
	logger   *zap.Logger
}

const (
	fakeIP uint32 = 1<<24 | 2<<16 | 3<<8 | 4

	fakeSpanDuration = 123 * time.Microsecond
)

func (w worker) simulateTraces() {
	tracer := opentracing.GlobalTracer()
	var i int
	for atomic.LoadUint32(w.running) == 1 {
		sp := tracer.StartSpan("lets-go")
		ext.SpanKindRPCClient.Set(sp)
		ext.PeerHostIPv4.Set(sp, fakeIP)
		ext.PeerService.Set(sp, "tracegen-server")
		if w.debug {
			ext.SamplingPriority.Set(sp, 100)
		}

		childCtx := sp.Context()
		if w.marshal {
			m := make(map[string]string)
			c := opentracing.TextMapCarrier(m)
			if err := tracer.Inject(sp.Context(), opentracing.TextMap, c); err == nil {
				c := opentracing.TextMapCarrier(m)
				childCtx, err = tracer.Extract(opentracing.TextMap, c)
				if err != nil {
					w.logger.Error("cannot extract from TextMap", zap.Error(err))
				}
			} else {
				w.logger.Error("cannot inject span", zap.Error(err))
			}
		}
		child := opentracing.StartSpan(
			"okey-dokey",
			ext.RPCServerOption(childCtx),
		)
		ext.PeerHostIPv4.Set(child, fakeIP)
		ext.PeerService.Set(child, "tracegen-client")

		time.Sleep(w.pause)

		if w.pause == 0 {
			child.Finish()
			sp.Finish()
		} else {
			opt := opentracing.FinishOptions{FinishTime: time.Now().Add(fakeSpanDuration)}
			child.FinishWithOptions(opt)
			sp.FinishWithOptions(opt)
		}

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
