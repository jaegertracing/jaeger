// Copyright (c) 2017 Uber Technologies, Inc.
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
	"fmt"
	"net/http"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/httpserver"
	"github.com/jaegertracing/jaeger/cmd/agent/app/processors"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
	jmetrics "github.com/jaegertracing/jaeger/pkg/metrics"
	zipkinThrift "github.com/jaegertracing/jaeger/thrift-gen/agent"
	jaegerThrift "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

const (
	defaultQueueSize     = 1000
	defaultMaxPacketSize = 65000
	defaultServerWorkers = 10

	defaultHTTPServerHostPort = ":5778"

	jaegerModel Model = "jaeger"
	zipkinModel       = "zipkin"

	compactProtocol Protocol = "compact"
	binaryProtocol           = "binary"
)

// Model used to distinguish the data transfer model
type Model string

// Protocol used to distinguish the data transfer protocol
type Protocol string

var (
	errNoReporters = errors.New("agent requires at least one Reporter")

	protocolFactoryMap = map[Protocol]thrift.TProtocolFactory{
		compactProtocol: thrift.NewTCompactProtocolFactory(),
		binaryProtocol:  thrift.NewTBinaryProtocolFactoryDefault(),
	}
)

// Builder Struct to hold configurations
type Builder struct {
	Processors []ProcessorConfiguration `yaml:"processors"`
	HTTPServer HTTPServerConfiguration  `yaml:"httpServer"`
	Metrics    jmetrics.Builder         `yaml:"metrics"`

	reporters      []reporter.Reporter
	metricsFactory metrics.Factory
	configManager  httpserver.ClientConfigManager
}

// ProcessorConfiguration holds config for a processor that receives spans from Server
type ProcessorConfiguration struct {
	Workers  int                 `yaml:"workers"`
	Model    Model               `yaml:"model"`
	Protocol Protocol            `yaml:"protocol"`
	Server   ServerConfiguration `yaml:"server"`
}

// ServerConfiguration holds config for a server that receives spans from the network
type ServerConfiguration struct {
	QueueSize     int    `yaml:"queueSize"`
	MaxPacketSize int    `yaml:"maxPacketSize"`
	HostPort      string `yaml:"hostPort" validate:"nonzero"`
}

// HTTPServerConfiguration holds config for a server providing sampling strategies and baggage restrictions to clients
type HTTPServerConfiguration struct {
	HostPort string `yaml:"hostPort" validate:"nonzero"`
}

// WithReporters adds auxiliary reporters.
func (b *Builder) WithReporters(r ...reporter.Reporter) *Builder {
	b.reporters = append(b.reporters, r...)
	return b
}

// WithMetricsFactory sets an externally initialized metrics factory.
func (b *Builder) WithMetricsFactory(mf metrics.Factory) *Builder {
	b.metricsFactory = mf
	return b
}

// GetMetricsFactory returns metrics factory used by the agent.
func (b *Builder) GetMetricsFactory() (metrics.Factory, error) {
	if b.metricsFactory != nil {
		return b.metricsFactory, nil
	}

	baseFactory, err := b.Metrics.CreateMetricsFactory("jaeger")
	if err != nil {
		return nil, err
	}

	b.metricsFactory = baseFactory.Namespace("agent", nil)
	return b.metricsFactory, nil
}

// CreateAgent creates the Agent
func (b *Builder) CreateAgent(logger *zap.Logger) (*Agent, error) {
	mFactory, err := b.GetMetricsFactory()
	if err != nil {
		return nil, errors.Wrap(err, "cannot create metrics factory")
	}
	r, err := b.getReporter(logger)
	if err != nil {
		return nil, err
	}
	processors, err := b.GetProcessors(r, mFactory, logger)
	if err != nil {
		return nil, err
	}
	server, err := b.HTTPServer.getHTTPServer(b.configManager, mFactory, &b.Metrics)
	if err != nil {
		return nil, err
	}
	return NewAgent(processors, server, logger), nil
}

func (b *Builder) getReporter(logger *zap.Logger) (reporter.Reporter, error) {
	if len(b.reporters) == 0 {
		return nil, errors.New("Missing required reporters")
	}
	return reporter.NewMultiReporter(b.reporters...), nil
}

// GetProcessors creates Processors with attached Reporter
func (b *Builder) GetProcessors(rep reporter.Reporter, mFactory metrics.Factory, logger *zap.Logger) ([]processors.Processor, error) {
	retMe := make([]processors.Processor, len(b.Processors))
	for idx, cfg := range b.Processors {
		protoFactory, ok := protocolFactoryMap[cfg.Protocol]
		if !ok {
			return nil, fmt.Errorf("cannot find protocol factory for protocol %v", cfg.Protocol)
		}
		var handler processors.AgentProcessor
		switch cfg.Model {
		case jaegerModel:
			handler = jaegerThrift.NewAgentProcessor(rep)
		case zipkinModel:
			handler = zipkinThrift.NewAgentProcessor(rep)
		default:
			return nil, fmt.Errorf("cannot find agent processor for data model %v", cfg.Model)
		}
		metrics := mFactory.Namespace("", map[string]string{
			"protocol": string(cfg.Protocol),
			"model":    string(cfg.Model),
		})
		processor, err := cfg.GetThriftProcessor(metrics, protoFactory, handler, logger)
		if err != nil {
			return nil, err
		}
		retMe[idx] = processor
	}
	return retMe, nil
}

// WithClientConfigManager adds configuration manager.
func (b *Builder) WithClientConfigManager(manager httpserver.ClientConfigManager) *Builder {
	b.configManager = manager
	return b
}

// GetHTTPServer creates an HTTP server that provides sampling strategies and baggage restrictions to client libraries.
func (c HTTPServerConfiguration) getHTTPServer(manager httpserver.ClientConfigManager, mFactory metrics.Factory, mBuilder *jmetrics.Builder) (*http.Server, error) {
	if manager == nil {
		return nil, errors.New("Missing required Client config manager")
	}
	if c.HostPort == "" {
		c.HostPort = defaultHTTPServerHostPort
	}
	server := httpserver.NewHTTPServer(c.HostPort, manager, mFactory)
	if h := mBuilder.Handler(); mFactory != nil && h != nil {
		server.Handler.(*http.ServeMux).Handle(mBuilder.HTTPRoute, h)
	}
	return server, nil
}

// GetThriftProcessor gets a TBufferedServer backed Processor using the collector configuration
func (c *ProcessorConfiguration) GetThriftProcessor(
	mFactory metrics.Factory,
	factory thrift.TProtocolFactory,
	handler processors.AgentProcessor,
	logger *zap.Logger,
) (processors.Processor, error) {
	c.applyDefaults()

	server, err := c.Server.getUDPServer(mFactory)
	if err != nil {
		return nil, err
	}

	return processors.NewThriftProcessor(server, c.Workers, mFactory, factory, handler, logger)
}

func (c *ProcessorConfiguration) applyDefaults() {
	c.Workers = defaultInt(c.Workers, defaultServerWorkers)
}

func (c *ServerConfiguration) applyDefaults() {
	c.QueueSize = defaultInt(c.QueueSize, defaultQueueSize)
	c.MaxPacketSize = defaultInt(c.MaxPacketSize, defaultMaxPacketSize)
}

// getUDPServer gets a TBufferedServer backed server using the server configuration
func (c *ServerConfiguration) getUDPServer(mFactory metrics.Factory) (servers.Server, error) {
	c.applyDefaults()

	if c.HostPort == "" {
		return nil, fmt.Errorf("no host:port provided for udp server: %+v", *c)
	}
	transport, err := thriftudp.NewTUDPServerTransport(c.HostPort)
	if err != nil {
		return nil, err
	}

	return servers.NewTBufferedServer(transport, c.QueueSize, c.MaxPacketSize, mFactory)
}

func defaultInt(value int, defaultVal int) int {
	if value == 0 {
		value = defaultVal
	}
	return value
}
