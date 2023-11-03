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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
)

// Mutex is just like the standard sync.Mutex, except that it is aware of the Context
// and logs some diagnostic information into the current span.
type Mutex struct {
	SessionBaggageKey string
	LogFactory        log.Factory

	realLock sync.Mutex
	holder   string

	waiters     []string
	waitersLock sync.Mutex
}

// Lock acquires an exclusive lock.
func (sm *Mutex) Lock(ctx context.Context) {
	logger := sm.LogFactory.For(ctx)
	session := BaggageItem(ctx, sm.SessionBaggageKey)
	activeSpan := trace.SpanFromContext(ctx)
	activeSpan.SetAttributes(attribute.String(sm.SessionBaggageKey, session))

	sm.waitersLock.Lock()
	if waiting := len(sm.waiters); waiting > 0 && activeSpan != nil {
		logger.Info(
			fmt.Sprintf("Waiting for lock behind %d transactions", waiting),
			zap.String("blockers", fmt.Sprintf("%v", sm.waiters)),
		)
	}
	sm.waiters = append(sm.waiters, session)
	sm.waitersLock.Unlock()

	sm.realLock.Lock()
	sm.holder = session

	sm.waitersLock.Lock()
	behindLen := len(sm.waiters) - 1
	behindIDs := fmt.Sprintf("%v", sm.waiters[1:]) // skip self
	sm.waitersLock.Unlock()

	logger.Info(
		fmt.Sprintf("Acquired lock; %d transactions waiting behind", behindLen),
		zap.String("waiters", behindIDs),
	)
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
