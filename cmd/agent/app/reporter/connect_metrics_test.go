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
	"time"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics/metricstest"
)

type connectMetricsTest struct {
	mb   *metricstest.Factory

}


func testConnectMetrics(fn func(tr *connectMetricsTest, r *ConnectMetricsReporter)) {
	testConnectMetricsWithParams(ConnectMetricsReporterParams{
	}, fn)
}

func testConnectMetricsWithParams(params ConnectMetricsReporterParams, fn func(tr *connectMetricsTest, r *ConnectMetricsReporter)) {
	mb := metricstest.NewFactory(time.Hour)
	params.MetricsFactory = mb
	r := WrapWithConnectMetrics(params)

	tr := &connectMetricsTest{
		mb:   mb,
	}

	fn(tr, r)
}

func testCollectorConnected(r *ConnectMetricsReporter)  {
	r.CollectorConnected("127.0.0.1:14250")
}

func testCollectorAborted(r *ConnectMetricsReporter)  {
	r.CollectorAborted("127.0.0.1:14250")
}


func TestConnectMetrics(t *testing.T) {

	testConnectMetrics(func(tr *connectMetricsTest, r *ConnectMetricsReporter) {
		getGauge := func() int64{
			_, gauges := tr.mb.Snapshot()
			return gauges["connection_status.connected_collector_status|target=127.0.0.1:14250"]
		}

		testCollectorAborted(r)
		assert.EqualValues(t,0, getGauge())

		testCollectorConnected(r)
		assert.EqualValues(t,1, getGauge())
	})
}