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

type connectMetrics struct {
	// used for reflect current connection stability
	Reconnects metrics.Counter `metric:"collector_reconnects" help:"Number of successful connections (including reconnects) to the collector."`

	// Connection status that jaeger-agent to jaeger-collector, 1 is connected, 0 is disconnected
	Status metrics.Gauge `metric:"collector_connected" help:"Status of connection between the agent and the collector; 1 is connected, 0 is disconnected"`
}

// ConnectMetricsParams include connectMetrics necessary params and connectMetrics, likes connectMetrics API
// If want to modify metrics of connectMetrics, must via ConnectMetrics API
type ConnectMetrics struct {
	Logger          *zap.Logger     // required
	MetricsFactory  metrics.Factory // required
	ExpireFrequency time.Duration
	ExpireTTL       time.Duration
	connectMetrics  *connectMetrics
}

// NewConnectMetrics will be initialize ConnectMetrics
func (r *ConnectMetrics) NewConnectMetrics() {
	if r.ExpireFrequency == 0 {
		r.ExpireFrequency = defaultExpireFrequency
	}
	if r.ExpireTTL == 0 {
		r.ExpireTTL = defaultExpireTTL
	}

	r.connectMetrics = new(connectMetrics)
	r.MetricsFactory = r.MetricsFactory.Namespace(metrics.NSOptions{Name: "connection_status"})
	metrics.MustInit(r.connectMetrics, r.MetricsFactory, nil)
}

// OnConnectionStatusChange used for pass the status parameter when connection is changed
// 0 is disconnected, 1 is connected
// For quick view status via use `sum(jaeger_agent_connection_status_collector_connected{}) by (instance) > bool 0`
func (r *ConnectMetrics) OnConnectionStatusChange(connected bool) {
	if connected {
		r.connectMetrics.Status.Update(1)
		r.connectMetrics.Reconnects.Inc(1)
	} else {
		r.connectMetrics.Status.Update(0)
	}
}
