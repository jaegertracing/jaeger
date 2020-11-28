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
	mb *metricstest.Factory
}

func testConnectMetrics(fn func(tr *connectMetricsTest, r *ConnectMetrics)) {
	testConnectMetricsWithParams(ConnectMetricsParams{}, fn)
}

func testConnectMetricsWithParams(params ConnectMetricsParams, fn func(tr *connectMetricsTest, r *ConnectMetrics)) {
	mb := metricstest.NewFactory(time.Hour)
	params.MetricsFactory = mb
	r := WrapWithConnectMetrics(params, "127.0.0.1:14250")

	tr := &connectMetricsTest{
		mb: mb,
	}

	fn(tr, r)
}

func testCollectorConnected(r *ConnectMetrics) {
	r.OnConnectionStatusChange(1)
}

func testCollectorAborted(r *ConnectMetrics) {
	r.OnConnectionStatusChange(0)
}

func TestConnectMetrics(t *testing.T) {

	testConnectMetrics(func(tr *connectMetricsTest, r *ConnectMetrics) {
		getGauge := func() map[string]int64 {
			_, gauges := tr.mb.Snapshot()
			return gauges
		}

		getCount := func() map[string]int64 {
			counts, _ := tr.mb.Snapshot()
			return counts
		}

		// testing connect aborted
		testCollectorAborted(r)
		assert.EqualValues(t, 0, getGauge()["connection_status.connected_collector_status|target=127.0.0.1:14250"])

		// testing connect connected
		testCollectorConnected(r)
		assert.EqualValues(t, 1, getGauge()["connection_status.connected_collector_status|target=127.0.0.1:14250"])
		assert.EqualValues(t, 1, getCount()["connection_status.connected_collector_reconnect"])

		// testing reconnect counts
		testCollectorAborted(r)
		testCollectorConnected(r)
		assert.EqualValues(t, 2, getCount()["connection_status.connected_collector_reconnect"])

	})
}
