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

	// Connection status that jaeger-agent to jaeger-collector, 1 is connected, 0 is disconnected
	ConnectedCollectorStatus metrics.Gauge `metric:"connected_collector_status" help:"Connection status that jaeger-agent to jaeger-collector, 1 is connected, 0 is disconnected"`
}

// ConnectMetricsParams include connectMetrics necessary params and connectMetrics, likes connectMetrics API
// If want to modify metrics of connectMetrics, must via ConnectMetrics API
type ConnectMetricsParams struct {
	Logger          *zap.Logger     // required
	MetricsFactory  metrics.Factory // required
	ExpireFrequency time.Duration
	ExpireTTL       time.Duration
	connectMetrics  *connectMetrics
}

// NewConnectMetrics creates ConnectMetricsParams.
func NewConnectMetrics(cmp ConnectMetricsParams) *ConnectMetricsParams {
	if cmp.ExpireFrequency == 0 {
		cmp.ExpireFrequency = defaultExpireFrequency
	}
	if cmp.ExpireTTL == 0 {
		cmp.ExpireTTL = defaultExpireTTL
	}

	cm := new(connectMetrics)
	cmp.MetricsFactory = cmp.MetricsFactory.Namespace(metrics.NSOptions{Name: "connection_status"})
	metrics.MustInit(cm, cmp.MetricsFactory, nil)
	cmp.connectMetrics = cm

	return &cmp
}

// OnConnectionStatusChange used for pass the status parameter when connection is changed
// 0 is disconnected, 1 is connected
// For quick view status via use `sum(jaeger_agent_connection_status_connected_collector_status{}) by (instance) > bool 0`
func (r *ConnectMetricsParams) OnConnectionStatusChange(connected bool) {
	if connected {
		r.connectMetrics.ConnectedCollectorStatus.Update(1)
		r.connectMetrics.ConnectedCollectorReconnect.Inc(1)
	} else {
		r.connectMetrics.ConnectedCollectorStatus.Update(0)
	}
}
