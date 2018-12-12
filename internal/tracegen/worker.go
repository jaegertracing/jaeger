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
	id       int             // worker id
	traces   int             // how many traces the worker has to generate (only when duration==0)
	marshall bool            // whether the worker needs to marshall trace context via HTTP headers
	debug    bool            // whether to set DEBUG flag on the spans
	duration time.Duration   // how long to run the test for (overrides `traces`)
	pause    time.Duration   // how long to pause before finishing the trace
	running  *uint32         // pointer to shared flag that indicates it's time to stop the test
	wg       *sync.WaitGroup // notify when done
	logger   *zap.Logger
}

var fakeIP uint32 = 1<<24 | 2<<16 | 3<<8 | 4

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
		if w.marshall {
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
			opt := opentracing.FinishOptions{FinishTime: time.Now().Add(123 * time.Microsecond)}
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
