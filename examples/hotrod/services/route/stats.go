// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package route

import (
	"context"
	"expvar"
	"time"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing"
)

var (
	routeCalcByCustomer = expvar.NewMap("route.calc.by.customer.sec")
	routeCalcBySession  = expvar.NewMap("route.calc.by.session.sec")
)

var stats = []struct {
	expvar     *expvar.Map
	baggageKey string
}{
	{
		expvar:     routeCalcByCustomer,
		baggageKey: "customer",
	},
	{
		expvar:     routeCalcBySession,
		baggageKey: "session",
	},
}

func updateCalcStats(ctx context.Context, delay time.Duration) {
	delaySec := float64(delay/time.Millisecond) / 1000.0
	for _, s := range stats {
		key := tracing.BaggageItem(ctx, s.baggageKey)
		if key != "" {
			s.expvar.AddFloat(key, delaySec)
		}
	}
}
