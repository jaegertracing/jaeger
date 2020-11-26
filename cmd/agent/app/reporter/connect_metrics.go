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

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

// Structure built for future code expansion or add new metrics.
type connectMetrics struct {
}

// ConnectMetricsReporterParams is used as input to WrapWithConnectMetrics.
type ConnectMetricsReporterParams struct {
	Logger          *zap.Logger     // required
	MetricsFactory  metrics.Factory // required
	ExpireFrequency time.Duration
	ExpireTTL       time.Duration
}

// ConnectMetricsReporter is a decorator also it not actual use currently.
// Structure built for future code expansion
type ConnectMetricsReporter struct {
	params         ConnectMetricsReporterParams
	connectMetrics *connectMetrics
	shutdown       chan struct{}
	closed         *atomic.Bool
}

// WrapWithConnectMetrics creates ConnectMetricsReporter.
func WrapWithConnectMetrics(params ConnectMetricsReporterParams) *ConnectMetricsReporter {
	if params.ExpireFrequency == 0 {
		params.ExpireFrequency = defaultExpireFrequency
	}
	if params.ExpireTTL == 0 {
		params.ExpireTTL = defaultExpireTTL
	}
	cm := new(connectMetrics)
	params.MetricsFactory = params.MetricsFactory.Namespace(metrics.NSOptions{Name: "connection_status"})
	metrics.MustInit(cm, params.MetricsFactory, nil)
	r := &ConnectMetricsReporter{
		params:         params,
		connectMetrics: cm,
		shutdown:       make(chan struct{}),
		closed:         atomic.NewBool(false),
	}
	return r
}

// CollectorConnected used for change metric as agent connected.
func (r *ConnectMetricsReporter) CollectorConnected(target string) {
	metric := r.params.MetricsFactory.Gauge(metrics.Options{
		Name: "connected_collector_status",
		Help: "Connection status that jaeger-agent to jaeger-collector, 1 is connected, 0 is disconnected",
		Tags: map[string]string{"target": target},
	})
	metric.Update(1)

}

// CollectorAborted used for change metric as agent disconnected.
func (r *ConnectMetricsReporter) CollectorAborted(target string) {
	metric := r.params.MetricsFactory.Gauge(metrics.Options{
		Name: "connected_collector_status",
		Help: "Connection status that jaeger-agent to jaeger-collector, 1 is connected, 0 is disconnected",
		Tags: map[string]string{"target": target},
	})
	metric.Update(0)
}
