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
		creditsPerSecond = 100
		iterations       = 20
		epsilon          = 1.5
	)
	ch := make(chan interface{})
	go func() {
		defer close(ch)
		for i := 0; i < iterations; i++ {
			ch <- time.Now()
		}
	}()

	rateLimiter := newRateLimiter(ch, creditsPerSecond)
	defer rateLimiter.Stop()

	minInterval := time.Duration(float64(time.Second.Nanoseconds())/creditsPerSecond*epsilon) * time.Nanosecond
	for v := range rateLimiter.C {
		tick := v.(time.Time)
		require.True(t, time.Since(tick) <= minInterval)
	}
}

func TestRateLimiterStopWithoutRead(t *testing.T) {
	const creditsPerSecond = 1

	ch := make(chan interface{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ch <- struct{}{}
	}()

	rateLimiter := newRateLimiter(ch, creditsPerSecond)
	wg.Wait()
	// Test that this does not block forever.
	rateLimiter.Stop()
}

func TestRateLimiterStopBeforeSourceSend(t *testing.T) {
	const (
		creditsPerSecond = 10.0
		epsilon          = 1.5
	)

	ch := make(chan interface{})
	rateLimiter := newRateLimiter(ch, creditsPerSecond)
	// Wait for first tick, then call stop.
	interval := time.Duration(float64(time.Second.Nanoseconds()) * epsilon / creditsPerSecond)
	time.Sleep(interval)
	// Test that this does not block forever.
	rateLimiter.Stop()
}

func TestRateLimiterStopBeforeFirstTick(t *testing.T) {
	const creditsPerSecond = 1

	ch := make(chan interface{})
	rateLimiter := newRateLimiter(ch, creditsPerSecond)
	startTime := time.Now()
	// Test that this does not wait for tick.
	rateLimiter.Stop()
	require.True(t, time.Since(startTime) < time.Second)
}

func TestRateLimiterNoLimit(t *testing.T) {
	const (
		creditsPerSecond = 0
		iterations       = 100
		epsilon          = 1.5
	)

	ch := make(chan interface{})
	go func() {
		defer close(ch)
		for i := 0; i < iterations; i++ {
			ch <- i
		}
	}()

	r := newRateLimiter(ch, creditsPerSecond)
	minInterval := time.Duration(float64(time.Second.Nanoseconds())/creditsPerSecond*epsilon) * time.Nanosecond
	tick := time.Now()
	for range r.C {
		tick = time.Now()
		require.True(t, time.Since(tick) >= minInterval)
	}
}
