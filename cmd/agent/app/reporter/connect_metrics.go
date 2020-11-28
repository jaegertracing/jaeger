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
	"go.uber.org/zap"
)

// connectMetrics is real metric, but not support to directly change, need via ConnectMetrics for changed
type connectMetrics struct {
	// used for reflect current connection stability
	ConnectedCollectorReconnect metrics.Counter `metric:"connected_collector_reconnect" help:"Default is 1, the metric can reflect current connection stability, as reconnect action increase the metric increase."`
}

// ConnectMetricsParams include connectMetrics necessary params
// Although some are not used, it use in the future
type ConnectMetricsParams struct {
	Logger          *zap.Logger     // required
	MetricsFactory  metrics.Factory // required
	ExpireFrequency time.Duration
	ExpireTTL       time.Duration

	// connection target
	Target string
}

// ConnectMetrics include ConnectMetricsParams and connectMetrics, likes connectMetrics API
// If want to modify metrics of connectMetrics, must via ConnectMetrics API
type ConnectMetrics struct {
	params         ConnectMetricsParams
	connectMetrics *connectMetrics
}

// WrapWithConnectMetrics creates ConnectMetrics.
func WrapWithConnectMetrics(params ConnectMetricsParams, target string) *ConnectMetrics {
	if params.ExpireFrequency == 0 {
		params.ExpireFrequency = defaultExpireFrequency
	}
	if params.ExpireTTL == 0 {
		params.ExpireTTL = defaultExpireTTL
	}
	params.Target = target
	cm := new(connectMetrics)

	params.MetricsFactory = params.MetricsFactory.Namespace(metrics.NSOptions{Name: "connection_status"})
	metrics.MustInit(cm, params.MetricsFactory, nil)
	r := &ConnectMetrics{
		params:         params,
		connectMetrics: cm,
	}
	return r
}

// When connection is changed, pass the status parameter
// 0 is disconnected, 1 is connected
// For quick view status via use `sum(jaeger_agent_connection_status_connected_collector_status{}) by (instance) > bool 0`
func (r *ConnectMetrics) OnConnectionStatusChange(status int64) {
	metric := r.params.MetricsFactory.Gauge(metrics.Options{
		Name: "connected_collector_status",
		Help: "Connection status that jaeger-agent to jaeger-collector, 1 is connected, 0 is disconnected",
		Tags: map[string]string{"target": r.params.Target},
	})
	metric.Update(status)
	if status == 1 {
		r.connectMetrics.ConnectedCollectorReconnect.Inc(1)
	}
}
