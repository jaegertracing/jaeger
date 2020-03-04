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

package app

import (
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/tchannel/collector/app/server"
)

// Collector returns tchannel collector.
type Collector struct {
	tchServer *tchannel.Channel
}

// Start starts the tchannel collector
func Start(
	serviceName string,
	cOpts *Options,
	logger *zap.Logger,
	spanHandlers *app.SpanHandlers,
	strategyStore strategystore.StrategyStore,
) (*Collector, error) {
	c := &Collector{}
	tchServer, err := server.StartThriftServer(&server.ThriftServerParams{
		ServiceName:          serviceName,
		Port:                 cOpts.CollectorPort,
		JaegerBatchesHandler: spanHandlers.JaegerBatchesHandler,
		ZipkinSpansHandler:   spanHandlers.ZipkinSpansHandler,
		StrategyStore:        strategyStore,
		Logger:               logger,
	})
	if err != nil {
		return c, err
	}
	c.tchServer = tchServer
	return c, nil
}

// Close the component and all its underlying dependencies.
func (c *Collector) Close() error {
	if c.tchServer != nil {
		c.tchServer.Close()
	}
	return nil
}
