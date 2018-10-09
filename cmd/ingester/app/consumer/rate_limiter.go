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
)

// rateLimiter uses a ticker to keep events evenly spaced, thereby providing
// rate limiting.
type rateLimiter struct {
	ticker      *time.Ticker
	tickChannel <-chan time.Time
	done        chan struct{}
}

// newRateLimiter constructs a rate limiter with the provided creditsPerSecond.
// If creditsPerSecond is less than zero, no rate limiting will be
// provided, and all calls to this object will return immediately. If
// creditsPerSecond is exactly zero, calls will block indefinitely or until Stop
// is called.
func newRateLimiter(creditsPerSecond float64) *rateLimiter {
	var r *rateLimiter
	if creditsPerSecond < 0 {
		// Receiving from a closed channel returns immediately, which is useful
		// for implementing non-rate-limiting behavior.
		closedChannel := make(chan time.Time)
		close(closedChannel)
		r = &rateLimiter{
			tickChannel: closedChannel,
			done:        make(chan struct{}),
		}
	} else if creditsPerSecond == 0 {
		r = &rateLimiter{
			tickChannel: make(<-chan time.Time),
			done:        make(chan struct{}),
		}
	} else {
		interval := time.Nanosecond * time.Duration(float64(time.Second.Nanoseconds())/creditsPerSecond)
		ticker := time.NewTicker(interval)
		r = &rateLimiter{
			ticker:      ticker,
			tickChannel: ticker.C,
			done:        make(chan struct{}),
		}
	}

	return r
}

// Acquire causes rateLimiter to block until next tick, or until Stop is called.
// Note: Acquire has no corresponding release operation.
func (r *rateLimiter) Acquire() {
	select {
	case <-r.tickChannel:
	case <-r.done:
	}
}

// Stop stops rateLimiter ticker and unblocks any blocked Acquire call.
func (r *rateLimiter) Stop() {
	if r.ticker != nil {
		r.ticker.Stop()
	}
	close(r.done)
}
