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

package consumer

import (
	"time"

	"github.com/uber/jaeger-lib/utils"
)

// RateLimiter exposes an interface for a blocking rate limiter.
type RateLimiter interface {
	Acquire()
}

type noopRateLimiter struct{}

// Acquire is a no-op in the noopRateLimiter.
func (r *noopRateLimiter) Acquire() {
}

type rateLimiter struct {
	rateLimiter utils.RateLimiter
}

// NewRateLimiter constructs a new rateLimiter.
func NewRateLimiter(creditsPerSecond, maxBalance float64) RateLimiter {
	if creditsPerSecond <= 0 {
		return &noopRateLimiter{}
	}
	r := &rateLimiter{
		rateLimiter: utils.NewRateLimiter(creditsPerSecond, maxBalance),
	}
	return r
}

// Acquire causes rateLimiter to spend a credit and return immediately,
// or block until a credit becomes available.
func (r rateLimiter) Acquire() {
	const cost = 1.0
	for {
		if r.rateLimiter.CheckCredit(cost) {
			return
		}
		time.Sleep(r.rateLimiter.DetermineWaitTime(cost))
	}
}
