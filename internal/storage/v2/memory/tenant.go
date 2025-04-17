// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"sync"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	v1 "github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// Tenant is an in-memory store of traces for a single tenant
type Tenant struct {
	sync.RWMutex
	ids        []pcommon.TraceID // ring buffer used to evict oldest traces
	traces     map[pcommon.TraceID]ptrace.Traces
	services   map[string]struct{}
	operations map[string]map[tracestore.Operation]struct{}
	config     *v1.Configuration
	evict      int // position in ids[] of the last evicted trace
}

func newTenant(cfg *v1.Configuration) *Tenant {
	return &Tenant{
		ids:        make([]pcommon.TraceID, cfg.MaxTraces),
		traces:     map[pcommon.TraceID]ptrace.Traces{},
		services:   map[string]struct{}{},
		operations: map[string]map[tracestore.Operation]struct{}{},
		config:     cfg,
		evict:      -1,
	}
}

func (t *Tenant) storeService(serviceName string) {
	t.Lock()
	defer t.Unlock()
	t.services[serviceName] = struct{}{}
}

func (t *Tenant) storeOperation(serviceName string, operation tracestore.Operation) {
	t.Lock()
	defer t.Unlock()
	if _, ok := t.operations[serviceName]; !ok {
		t.operations[serviceName] = make(map[tracestore.Operation]struct{})
		t.operations[serviceName][operation] = struct{}{}
	} else {
		t.operations[serviceName][operation] = struct{}{}
	}
}

func (t *Tenant) storeTraces(traceId pcommon.TraceID, resourceSpanSlice ptrace.ResourceSpansSlice) {
	t.Lock()
	defer t.Unlock()
	if foundTraces, ok := t.traces[traceId]; !ok {
		traces := ptrace.NewTraces()
		resourceSpanSlice.MoveAndAppendTo(traces.ResourceSpans())
		t.traces[traceId] = traces
		// if we have a limit, let's cleanup the oldest traces
		if t.config.MaxTraces > 0 {
			// we only have to deal with this slice if we have a limit
			t.evict = (t.evict + 1) % t.config.MaxTraces
			// do we have an item already on this position? if so, we are overriding it,
			// and we need to remove from the map
			if !t.ids[t.evict].IsEmpty() {
				delete(t.traces, t.ids[t.evict])
			}
			// update the ring with the trace id
			t.ids[t.evict] = traceId
		}
	} else {
		resourceSpanSlice.MoveAndAppendTo(foundTraces.ResourceSpans())
	}
}
