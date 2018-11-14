// Copyright (c) 2018 The Jaeger Authors.
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

package tchannel

import (
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/httpserver"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
)

// ProxyBuilder holds objects communicating with collector
type ProxyBuilder struct {
	reporter reporter.Reporter
	manager  httpserver.ClientConfigManager
}

// NewCollectorProxy creates ProxyBuilder
func NewCollectorProxy(builder *Builder, mFactory metrics.Factory, logger *zap.Logger) (*ProxyBuilder, error) {
	b := &ProxyBuilder{}
	r, err := builder.CreateReporter(logger)
	if err != nil {
		return nil, err
	}
	tchannelMetrics := mFactory.Namespace("", map[string]string{"protocol": "tchannel"})
	b.reporter = reporter.WrapWithMetrics(r, tchannelMetrics)
	b.manager = httpserver.WrapWithMetrics(httpserver.NewCollectorProxy(r.CollectorServiceName(), r.Channel()), tchannelMetrics)
	return b, nil
}

// GetReporter returns Reporter
func (b ProxyBuilder) GetReporter() reporter.Reporter {
	return b.reporter
}

// GetManager returns manager
func (b ProxyBuilder) GetManager() httpserver.ClientConfigManager {
	return b.manager
}

// Close closes connections used by proxy.
func (b ProxyBuilder) Close() error {
	b.reporter.Channel().Close()
	return nil
}
