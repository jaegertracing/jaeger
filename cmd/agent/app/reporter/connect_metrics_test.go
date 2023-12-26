// Copyright (c) 2020 The Jaeger Authors.
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
