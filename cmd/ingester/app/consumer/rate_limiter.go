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
	"time"
)

// rateLimiter wraps a channel to provide rate limiting.
type rateLimiter struct {
	C           chan interface{}
	ticker      *time.Ticker
	tickChannel <-chan time.Time
	source      <-chan interface{}
	done        chan struct{}
	wg          sync.WaitGroup
}

// Create a new rate limiter for a channel. If creditsPerSecond is not greater
// than zero, newRateLimiter will use simply wrap the original channel and
// provide no rate limiting. A ticker is used to implement a rate limiter by
// waiting on a tickChannel before each read from the original channel.
// N.B. The ticker is not guaranteed to queue accumulated ticks during a read
// from the original channel. As the Go doc for time.NewTicker points out:
// "It adjusts the intervals or drops ticks to make up for slow receivers."
func newRateLimiter(source <-chan interface{}, creditsPerSecond float64) *rateLimiter {
	var r *rateLimiter
	if creditsPerSecond <= 0 {
		// Receiving from a closed channel returns immediately, which is useful
		// for implementing non-rate-limiting behavior.
		closedChannel := make(chan time.Time)
		close(closedChannel)
		r = &rateLimiter{
			C:           make(chan interface{}),
			tickChannel: closedChannel,
			source:      source,
			done:        make(chan struct{}),
		}
	} else {
		interval := time.Nanosecond * time.Duration(float64(time.Second.Nanoseconds())/creditsPerSecond)
		ticker := time.NewTicker(interval)
		r = &rateLimiter{
			C:           make(chan interface{}),
			ticker:      ticker,
			tickChannel: ticker.C,
			source:      source,
			done:        make(chan struct{}),
		}
	}

	r.wg.Add(1)
	go func() {
		defer close(r.C)
		defer r.wg.Done()
		for {
			// This select block is a bit ugly, but it protects against three
			// blocking calls that could cause the Stop method to hang
			// unnecessarily:
			// 1. The tick has not yet occurred (avoids wait for first tick).
			// 2. The writer has not yet submitted an event to r.source.
			// 3. The reader has not yet consumed an event from r.C.
			select { // #1
			case <-r.tickChannel:
				select { // #2
				case v, ok := <-r.source:
					if !ok {
						return
					}

					select { // #3
					case r.C <- v:
						// Do nothing
					case <-r.done:
						return
					}
				case <-r.done:
					return
				}
			case <-r.done:
				return
			}
		}
	}()

	return r
}

// Stop stops ticker. Must be called in order to avoid a goroutine leak.
func (r *rateLimiter) Stop() {
	if r.ticker != nil {
		r.ticker.Stop()
	}
	close(r.done)
	r.wg.Wait()
}
