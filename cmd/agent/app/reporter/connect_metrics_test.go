// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"expvar"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/metricstest"
)

func TestConnectMetrics(t *testing.T) {
	mf := metricstest.NewFactory(time.Hour)
	defer mf.Stop()
	cm := NewConnectMetrics(mf)

	getGauge := func() map[string]int64 {
		_, gauges := mf.Snapshot()
		return gauges
	}

	getCount := func() map[string]int64 {
		counts, _ := mf.Snapshot()
		return counts
	}

	// no connection
	cm.OnConnectionStatusChange(false)
	assert.EqualValues(t, 0, getGauge()["connection_status.collector_connected"])

	// first connection
	cm.OnConnectionStatusChange(true)
	assert.EqualValues(t, 1, getGauge()["connection_status.collector_connected"])
	assert.EqualValues(t, 1, getCount()["connection_status.collector_reconnects"])

	// reconnect
	cm.OnConnectionStatusChange(false)
	cm.OnConnectionStatusChange(true)
	assert.EqualValues(t, 2, getCount()["connection_status.collector_reconnects"])

	cm.RecordTarget("collector-host")
	assert.Equal(t, `"collector-host"`, expvar.Get("gRPCTarget").String())

	// since expvars are singletons, the second constructor should grab the same var
	cm2 := NewConnectMetrics(mf)
	assert.Same(t, cm.target, cm2.target)
}
