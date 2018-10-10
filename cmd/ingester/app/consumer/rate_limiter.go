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
	"math"
	"time"
)

const cost = 1.0

type rateLimiter struct {
	creditsPerSecond float64
	maxBalance       float64
	balance          float64
	lastTime         time.Time
	done             chan struct{}
}

// newRateLimiter constructs a new rateLimiter.
func newRateLimiter(creditsPerSecond, maxBalance float64) *rateLimiter {
	r := &rateLimiter{
		creditsPerSecond: creditsPerSecond,
		maxBalance:       maxBalance,
		balance:          maxBalance,
		done:             make(chan struct{}),
	}
	r.lastTime = time.Now()
	return r
}

// Acquire causes rateLimiter to spend a credit and return immediately,
// or block until a credit becomes available. Returns true if credits were
// acquired. Returns false if Stop was called before credits were acquired,
// interrupting the Acquire call.
// Note: Acquire has no corresponding release operation.
// N.B. This rate limiter implementation is not thread-safe. Do not call Acquire
// from more than one goroutine.
func (r *rateLimiter) Acquire() bool {
	for r.updateBalance() < cost {
		timer := time.NewTimer(r.calculateWait())
		select {
		case <-timer.C:
		case <-r.done:
			timer.Stop()
			return false
		}
	}
	r.balance -= cost
	return true
}

// Stop stops any ongoing Acquire operation, which exits immediately.
func (r *rateLimiter) Stop() {
	close(r.done)
}

func (r *rateLimiter) updateBalance() float64 {
	if r.balance == r.maxBalance {
		return r.balance
	}
	now := time.Now()
	interval := now.Sub(r.lastTime)
	r.balance += math.Min(interval.Seconds()*r.creditsPerSecond, r.maxBalance)
	r.lastTime = now
	return r.balance
}

func (r *rateLimiter) calculateWait() time.Duration {
	creditsNeeded := cost - r.balance
	waitTime := time.Nanosecond * time.Duration(float64(time.Second.Nanoseconds())*creditsNeeded/r.creditsPerSecond)
	return waitTime
}
