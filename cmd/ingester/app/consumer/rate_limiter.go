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

type config struct {
	creditsPerSecond float64
	maxBalance       float64
}

type rateLimiter struct {
	config
	balance  float64
	lastTime time.Time
	event    chan config
}

// newRateLimiter constructs a new rateLimiter.
func newRateLimiter(creditsPerSecond, maxBalance float64) *rateLimiter {
	r := &rateLimiter{
		config: config{
			creditsPerSecond: creditsPerSecond,
			maxBalance:       maxBalance,
		},
		balance: maxBalance,
		event:   make(chan config),
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
		case cfg, ok := <-r.event:
			timer.Stop()
			if !ok {
				return false
			}
			r.maxBalance = cfg.maxBalance
			r.creditsPerSecond = cfg.creditsPerSecond
		}
	}
	r.balance -= cost
	return true
}

// Stop stops any ongoing Acquire operation, which exits immediately.
func (r *rateLimiter) Stop() {
	close(r.event)
}

// Update replaces the current rate limiter configuration in a thread-safe
// manner. This function will panic if Stop has already been called on the
// rateLimiter.
func (r *rateLimiter) Update(creditsPerSecond, maxBalance float64) {
	r.event <- config{
		creditsPerSecond: creditsPerSecond,
		maxBalance:       maxBalance,
	}
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
