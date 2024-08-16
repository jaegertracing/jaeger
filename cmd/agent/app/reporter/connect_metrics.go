// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"expvar"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

type connectMetrics struct {
	// used for reflect current connection stability
	Reconnects metrics.Counter `metric:"collector_reconnects" help:"Number of successful connections (including reconnects) to the collector."`

	// Connection status that jaeger-agent to jaeger-collector, 1 is connected, 0 is disconnected
	Status metrics.Gauge `metric:"collector_connected" help:"Status of connection between the agent and the collector; 1 is connected, 0 is disconnected"`
}

// ConnectMetrics include connectMetrics necessary params if want to modify metrics of connectMetrics, must via ConnectMetrics API
type ConnectMetrics struct {
	metrics connectMetrics
	target  *expvar.String
}

// NewConnectMetrics will be initialize ConnectMetrics
func NewConnectMetrics(mf metrics.Factory) *ConnectMetrics {
	cm := &ConnectMetrics{}
	metrics.MustInit(&cm.metrics, mf.Namespace(metrics.NSOptions{Name: "connection_status"}), nil)

	if r := expvar.Get("gRPCTarget"); r == nil {
		cm.target = expvar.NewString("gRPCTarget")
	} else {
		cm.target = r.(*expvar.String)
	}

	return cm
}

// OnConnectionStatusChange used for pass the status parameter when connection is changed
// 0 is disconnected, 1 is connected
// For quick view status via use `sum(jaeger_agent_connection_status_collector_connected{}) by (instance) > bool 0`
func (cm *ConnectMetrics) OnConnectionStatusChange(connected bool) {
	if connected {
		cm.metrics.Status.Update(1)
		cm.metrics.Reconnects.Inc(1)
	} else {
		cm.metrics.Status.Update(0)
	}
}

// RecordTarget writes the current connection target to an expvar field.
func (cm *ConnectMetrics) RecordTarget(target string) {
	cm.target.Set(target)
}
