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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics/metricstest"
)

type connectMetricsTest struct {
	mf *metricstest.Factory
}

func testConnectMetrics(fn func(tr *connectMetricsTest, r *ConnectMetrics)) {
	testConnectMetricsWithParams(&ConnectMetrics{}, fn)
}

func testConnectMetricsWithParams(cm *ConnectMetrics, fn func(tr *connectMetricsTest, r *ConnectMetrics)) {
	mf := metricstest.NewFactory(time.Hour)
	cm.MetricsFactory = mf
	cm.NewConnectMetrics()

	tr := &connectMetricsTest{
		mf: mf,
	}

	fn(tr, cm)
}

func testCollectorConnected(r *ConnectMetrics) {
	r.OnConnectionStatusChange(true)
}

func testCollectorAborted(r *ConnectMetrics) {
	r.OnConnectionStatusChange(false)
}

func TestConnectMetrics(t *testing.T) {

	testConnectMetrics(func(tr *connectMetricsTest, r *ConnectMetrics) {
		getGauge := func() map[string]int64 {
			_, gauges := tr.mf.Snapshot()
			return gauges
		}

		getCount := func() map[string]int64 {
			counts, _ := tr.mf.Snapshot()
			return counts
		}

		// testing connect aborted
		testCollectorAborted(r)
		assert.EqualValues(t, 0, getGauge()["connection_status.collector_connected"])

		// testing connect connected
		testCollectorConnected(r)
		assert.EqualValues(t, 1, getGauge()["connection_status.collector_connected"])
		assert.EqualValues(t, 1, getCount()["connection_status.collector_reconnects"])

		// testing reconnect counts
		testCollectorAborted(r)
		testCollectorConnected(r)
		assert.EqualValues(t, 2, getCount()["connection_status.collector_reconnects"])

	})
}
