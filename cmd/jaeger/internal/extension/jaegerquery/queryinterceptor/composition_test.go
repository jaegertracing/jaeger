// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	pub "github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// TestInterceptorRunsWithTheFilterGateOff pins the interaction between the two opt-ins: an
// interceptor is configured, jaeger.query.structuredFilters is off, and a caller sends the
// legacy predicate fields. The interceptor still sees every predicate as a filter and can
// narrow it, because the query service's gate guards a filter a *caller* sent, while the shape
// the interceptor works in is made below the query service, in the reader decorator. Anyone
// running an interceptor therefore needs no second opt-in, and the deployment still accepts no
// filter over api_v3.
func TestInterceptorRunsWithTheFilterGateOff(t *testing.T) {
	disableStructuredFilters(t)

	var seen pub.Query
	next := &fakeReader{batch: tracesWith("k", "v")}
	decorated := NewReaderDecorator(next, fakeInterceptor{
		onQuery: func(q pub.Query) (pub.Query, error) {
			seen = q
			q.Filter = serviceFilter("gated")
			return q, nil
		},
	})
	next.capabilities = tracestore.SearchCapabilities{WithoutServiceName: true}

	qs := querysvc.NewQueryService(decorated, nil, querysvc.QueryServiceOptions{})
	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")
	_, err := jiter.FlattenWithErrors(qs.FindTraces(t.Context(), querysvc.TraceQueryParams{
		TraceQueryParams: tracestore.TraceQueryParams{
			ServiceName:  "original",
			Attributes:   attrs,
			StartTimeMin: time.Now().Add(-time.Hour),
			StartTimeMax: time.Now(),
		},
	}))
	require.NoError(t, err)

	require.NotNil(t, seen.Filter, "the interceptor is shown predicates, gate or no gate")
	assert.Equal(t, expression.OpAnd, seen.Filter.Op)
	assert.Len(t, seen.Filter.Args, 2, "the service and the tag both became predicates")
	assert.Equal(t, "gated", next.gotQuery.ServiceName, "storage gets what the interceptor chose")
	assert.Nil(t, next.gotQuery.Filter, "in the shape this backend supports")
}

// disableStructuredFilters turns the filter gate off for one test and restores it afterwards,
// so the test sets up the condition it is about rather than relying on the gate's default.
func disableStructuredFilters(t *testing.T) {
	gate := querysvc.StructuredFiltersGate
	original := gate.IsEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(gate.ID(), false))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(gate.ID(), original))
	})
}
