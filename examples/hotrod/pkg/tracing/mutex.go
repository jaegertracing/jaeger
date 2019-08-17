// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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
	holder   string

	waiters     []string
	waitersLock sync.Mutex
}

// Lock acquires an exclusive lock.
func (sm *Mutex) Lock(ctx context.Context) {
	var session string
	activeSpan := opentracing.SpanFromContext(ctx)
	if activeSpan != nil {
		session = activeSpan.BaggageItem(sm.SessionBaggageKey)
		activeSpan.SetTag(sm.SessionBaggageKey, session)
	}

	sm.waitersLock.Lock()
	if waiting := len(sm.waiters); waiting > 0 && activeSpan != nil {
		activeSpan.LogFields(
			log.String("event", fmt.Sprintf("Waiting for lock behind %d transactions", waiting)),
			log.String("blockers", fmt.Sprintf("%v", sm.waiters))) // avoid deferred slice.String()
		fmt.Printf("%s Waiting for lock behind %d transactions: %v\n", session, waiting, sm.waiters)
	}
	sm.waiters = append(sm.waiters, session)
	sm.waitersLock.Unlock()

	sm.realLock.Lock()
	sm.holder = session

	sm.waitersLock.Lock()
	behindLen := len(sm.waiters) - 1
	sm.waitersLock.Unlock()

	if activeSpan != nil {
		activeSpan.LogFields(log.String("event",
			fmt.Sprintf("Acquired lock with %d transactions waiting behind", behindLen)))
	}
}

// Unlock releases the lock.
func (sm *Mutex) Unlock() {
	sm.waitersLock.Lock()
	for i, v := range sm.waiters {
		if v == sm.holder {
			sm.waiters = append(sm.waiters[0:i], sm.waiters[i+1:]...)
			break
		}
	}
	sm.waitersLock.Unlock()

	sm.realLock.Unlock()
}
