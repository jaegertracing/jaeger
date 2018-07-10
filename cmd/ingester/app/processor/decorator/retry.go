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

package decorator

import (
	"io"
	"math/rand"
	"time"

	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
)

type retryDecorator struct {
	processor     processor.SpanProcessor
	retryAttempts metrics.Counter
	exhausted     metrics.Counter
	options       retryOptions
	io.Closer
}

// RetryOption allows setting options for exponential backoff retried
type RetryOption func(*retryOptions)

type retryOptions struct {
	minInterval, maxInterval time.Duration
	maxAttempts              uint
	propagateError           bool
	rand                     randInt63
}

type randInt63 interface {
	Int63n(int64) int64
}

var defaultOpts = retryOptions{
	minInterval:    time.Second,
	maxInterval:    1 * time.Minute,
	maxAttempts:    10,
	propagateError: false,
	rand:           rand.New(rand.NewSource(time.Now().UnixNano())),
}

// MinBackoffInterval sets the minimum backoff interval
func MinBackoffInterval(t time.Duration) RetryOption {
	return func(opt *retryOptions) {
		opt.minInterval = t
	}
}

// MaxAttempts sets the maximum number of attempts to retry
func MaxAttempts(attempts uint) RetryOption {
	return func(opt *retryOptions) {
		opt.maxAttempts = attempts
	}
}

// MaxBackoffInterval sets the maximum backoff interval
func MaxBackoffInterval(t time.Duration) RetryOption {
	return func(opt *retryOptions) {
		opt.maxInterval = t
	}
}

// Rand sets a random number generator
func Rand(r randInt63) RetryOption {
	return func(opt *retryOptions) {
		opt.rand = r
	}
}

// PropagateError sets whether to propagate errors when retries are exhausted
func PropagateError(b bool) RetryOption {
	return func(opt *retryOptions) {
		opt.propagateError = b
	}
}

// NewRetryingProcessor returns a processor that retries failures using an exponential backoff
// with jitter.
func NewRetryingProcessor(f metrics.Factory, processor processor.SpanProcessor, opts ...RetryOption) processor.SpanProcessor {
	options := defaultOpts
	for _, opt := range opts {
		opt(&options)
	}

	m := f.Namespace("span-processor", nil)
	return &retryDecorator{
		retryAttempts: m.Counter("retry-attempts", nil),
		exhausted:     m.Counter("retry-exhausted", nil),
		processor:     processor,
		options:       options,
	}
}

func (d *retryDecorator) Process(message processor.Message) error {
	err := d.processor.Process(message)

	if err == nil {
		return nil
	}

	for attempts := uint(0); err != nil && d.options.maxAttempts > attempts; attempts++ {
		time.Sleep(d.computeInterval(attempts))
		err = d.processor.Process(message)
		d.retryAttempts.Inc(1)
	}

	if err != nil {
		d.exhausted.Inc(1)
		if d.options.propagateError {
			return err
		}
	}

	return nil
}

func (d *retryDecorator) computeInterval(attempts uint) time.Duration {
	dur := (1 << attempts) * d.options.minInterval.Nanoseconds()
	if dur <= 0 || dur > d.options.maxInterval.Nanoseconds() {
		dur = d.options.maxInterval.Nanoseconds()
	}
	return time.Duration(d.options.rand.Int63n(dur))
}
