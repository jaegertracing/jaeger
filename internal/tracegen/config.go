package tracegen

import (
	"flag"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Config describes the test scenario.
type Config struct {
	Workers  int
	Traces   int
	Marshall bool
	Debug    bool
	Pause    time.Duration
	Duration time.Duration
}

// Flags registers config flags.
func (c *Config) Flags(fs *flag.FlagSet) {
	fs.IntVar(&c.Workers, "workers", 1, "Number of workers (goroutines) to run")
	fs.IntVar(&c.Traces, "traces", 1, "Number of traces to generate in each worker (ignored if duration is provided")
	fs.BoolVar(&c.Marshall, "marshall", false, "Whether to marshall trace context via HTTP headers")
	fs.BoolVar(&c.Debug, "debug", false, "Whether to set DEBUG flag on the spans to prevent downsampling")
	fs.DurationVar(&c.Pause, "pause", time.Microsecond, "How long to pause before finishing trace")
	fs.DurationVar(&c.Duration, "duration", 0, "For how long to run the test")
}

// Run executes the test scenario.
func (c *Config) Run(logger *zap.Logger) error {
	if c.Duration > 0 {
		c.Traces = 0
	} else if c.Traces <= 0 {
		return fmt.Errorf("Either `traces` or `duration` must be greater than 0")
	}

	wg := &sync.WaitGroup{}
	var running uint32 = 1
	for i := 0; i < c.Workers; i++ {
		wg.Add(1)
		w := worker{
			id:       i,
			traces:   c.Traces,
			marshall: c.Marshall,
			debug:    c.Debug,
			pause:    c.Pause,
			duration: c.Duration,
			running:  &running,
			wg:       wg,
			logger:   logger.With(zap.Int("worker", i)),
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
