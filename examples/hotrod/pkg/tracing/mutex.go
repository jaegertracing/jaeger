// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tracing

import (
	"context"
	"fmt"
	"sync"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

// Mutex is just like the standard sync.Mutex, except that it is aware of the Context
// and logs some diagnostic information into the current span.
type Mutex struct {
	SessionBaggageKey string

	realLock sync.Mutex

	waiters     []string
	waitersmock sync.Mutex
}

// Lock acquires an exclusive lock.
func (sm *Mutex) Lock(ctx context.Context) {
	var session string
	activeSpan := opentracing.SpanFromContext(ctx)
	if activeSpan != nil {
		session = activeSpan.BaggageItem(sm.SessionBaggageKey)
		activeSpan.SetTag(sm.SessionBaggageKey, session)
	}

	sm.waitersmock.Lock()
	if waiting := len(sm.waiters); waiting > 0 && activeSpan != nil {
		activeSpan.LogFields(
			log.String("event", fmt.Sprintf("Waiting for lock behind %d transactions", waiting)),
			log.Object("blockers", sm.waiters))
	}
	sm.waiters = append(sm.waiters, session)
	sm.waitersmock.Unlock()

	sm.realLock.Lock()

	sm.waitersmock.Lock()
	behindLen := len(sm.waiters) - 1
	sm.waitersmock.Unlock()

	if activeSpan != nil {
		activeSpan.LogEvent(
			fmt.Sprintf("Acquired lock with %d transactions waiting behind", behindLen))
	}
}

// Unlock releases the lock.
func (sm *Mutex) Unlock() {
	sm.waitersmock.Lock()
	sm.waiters = sm.waiters[0 : len(sm.waiters)-1]
	sm.waitersmock.Unlock()

	sm.realLock.Unlock()
}
