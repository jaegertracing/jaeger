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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRateLimiter(t *testing.T) {
	const (
		creditsPerSecond = 1000
		iterations       = 10
	)

	// Ticks cannot be timed precisely because time.Ticker reserves the right to
	// change the rate of ticks as it deems necessary (slows consumers, etc.).
	// Therefore, this code asserts that the tick is no more than 50% off the
	// expected time.
	idealInterval := time.Duration(float64(time.Second.Nanoseconds())/creditsPerSecond) * time.Nanosecond
	minInterval := time.Duration(float64(idealInterval.Nanoseconds())*0.5) * time.Nanosecond
	maxInterval := time.Duration(float64(idealInterval.Nanoseconds())*1.5) * time.Nanosecond

	rateLimiter := newRateLimiter(creditsPerSecond)
	defer rateLimiter.Stop()
	acquireStart := time.Now()
	for i := 0; i < iterations; i++ {
		rateLimiter.Acquire()
		actualInterval := time.Since(acquireStart)
		require.Truef(t, actualInterval >= minInterval, "Interval is too small %v < %v", actualInterval, minInterval)
		require.Truef(t, actualInterval <= maxInterval, "Interval is too large %v > %v", actualInterval, maxInterval)
		acquireStart = time.Now()
	}
}

func TestRateLimiterNoLimit(t *testing.T) {
	const (
		creditsPerSecond = -1
		iterations       = 100
	)

	r := newRateLimiter(creditsPerSecond)
	defer r.Stop()

	const maxInterval = time.Millisecond
	acquireStart := time.Now()
	for i := 0; i < iterations; i++ {
		acquireEnd := time.Now()
		interval := acquireEnd.Sub(acquireStart)
		require.Truef(t, interval <= maxInterval, "Expected no significant delay before first receive, actual delay = %v, max expected delay = %v", interval, maxInterval)
		acquireStart = time.Now()
	}
}

func TestRateLimiterInfiniteLimit(t *testing.T) {
	const (
		creditsPerSecond = 0
		sleepInterval    = time.Millisecond
	)
	r := newRateLimiter(creditsPerSecond)
	var wg sync.WaitGroup
	wg.Add(1)
	var m sync.Mutex
	var stopped bool
	go func() {
		wg.Done()
		time.Sleep(sleepInterval)
		r.Stop()
		m.Lock()
		defer m.Unlock()
		stopped = true
	}()

	wg.Wait()
	r.Acquire()
	m.Lock()
	defer m.Unlock()
	require.Truef(t, stopped, "Acquire should not return when creditsPerSecond is zero unless Stop is invoked")
}
