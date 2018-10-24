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
	"time"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	"github.com/jaegertracing/jaeger/internal/utils"
)

type rateLimitDecorator struct {
	processor   processor.SpanProcessor
	rateLimiter utils.RateLimiter
	io.Closer
}

// RateLimitOption allows setting options for rate limiting.
type RateLimitOption func(*rateLimitOptions)

type rateLimitOptions struct {
	creditsPerSecond float64
	maxBalance       float64
}

var defaultRateLimitOpts = rateLimitOptions{
	creditsPerSecond: 100,
	maxBalance:       100,
}

// CreditsPerSecond sets the number of credits accrued per second in the rate limiter.
func CreditsPerSecond(creditsPerSecond float64) RateLimitOption {
	return func(opt *rateLimitOptions) {
		opt.creditsPerSecond = creditsPerSecond
	}
}

// MaxBalance sets the maximum number of credits that may be accrued in the rate limiter.
func MaxBalance(maxBalance float64) RateLimitOption {
	return func(opt *rateLimitOptions) {
		opt.maxBalance = maxBalance
	}
}

// NewRateLimitingProcessor returns a processor that retries failures using an exponential backoff
// with jitter.
func NewRateLimitingProcessor(processor processor.SpanProcessor, opts ...RateLimitOption) processor.SpanProcessor {
	options := defaultRateLimitOpts
	for _, opt := range opts {
		opt(&options)
	}
	return &rateLimitDecorator{
		processor:   processor,
		rateLimiter: utils.NewRateLimiter(options.creditsPerSecond, options.maxBalance),
	}
}

func (d *rateLimitDecorator) Process(message processor.Message) error {
	const cost = 1
	if _, waitTime := d.rateLimiter.CheckCredit(cost); waitTime > 0 {
		time.Sleep(waitTime)
	}
	return d.processor.Process(message)
}
