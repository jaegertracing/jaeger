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
		maxBalance       = 1
		iterations       = 10
	)

	// Timing is not precise. Using 50% as maximum error.
	idealInterval := time.Duration(float64(time.Second.Nanoseconds())/creditsPerSecond) * time.Nanosecond
	minInterval := time.Duration(float64(idealInterval.Nanoseconds())*0.50) * time.Nanosecond
	maxInterval := time.Duration(float64(idealInterval.Nanoseconds())*1.50) * time.Nanosecond

	rateLimiter := NewRateLimiter(creditsPerSecond, maxBalance)
	defer rateLimiter.Stop()
	acquireStart := time.Now()
	for i := 0; i < iterations; i++ {
		ok := rateLimiter.Acquire()
		require.True(t, ok)
		if i == 0 {
			// Skip first iteration, when balance is full.
			acquireStart = time.Now()
			continue
		}
		actualInterval := time.Since(acquireStart)
		require.Truef(t, actualInterval >= minInterval, "Interval is too small %v < %v", actualInterval, minInterval)
		require.Truef(t, actualInterval <= maxInterval, "Interval is too large %v > %v", actualInterval, maxInterval)
		acquireStart = time.Now()
	}
}

func TestRateLimiterReturnsFalseWhenInterrupted(t *testing.T) {
	const (
		creditsPerSecond = 0.01
		maxBalance       = 1
	)

	r := NewRateLimiter(creditsPerSecond, maxBalance)

	// Empty token bucket.
	ok := r.Acquire()
	require.True(t, ok)

	// Stop rateLimiter while Acquire is in progress.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
		ok = r.Acquire()
		require.False(t, ok)
	}()
	wg.Wait()
	r.Stop()
}

func TestRateLimiterBurst(t *testing.T) {
	const (
		creditsPerSecond = 1000.0
		maxBurst         = 10.0
	)

	r := NewRateLimiter(creditsPerSecond, maxBurst)

	expectedWaitTimePerCredit := time.Nanosecond * time.Duration(creditsPerSecond*time.Second.Nanoseconds())
	expectedWaitTimeTotal := expectedWaitTimePerCredit * maxBurst
	startBurst := time.Now()
	for i := 0; i < maxBurst; i++ {
		ok := r.Acquire()
		require.True(t, ok)
	}
	actualWaitTime := time.Since(startBurst)
	require.Truef(t, actualWaitTime < expectedWaitTimeTotal, "Burst happened too slowly, expected wait time = less than %v, actual wait time = %v\n", expectedWaitTimeTotal, actualWaitTime)
}
