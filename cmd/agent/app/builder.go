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
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/httpserver"
	"github.com/jaegertracing/jaeger/cmd/agent/app/processors"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	tchreporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
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
	defaultMinPeers      = 3

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

	tchreporter.Builder `yaml:",inline"`

	otherReporters []reporter.Reporter
	metricsFactory metrics.Factory
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

// WithReporter adds auxiliary reporters.
func (b *Builder) WithReporter(r reporter.Reporter) *Builder {
	b.otherReporters = append(b.otherReporters, r)
	return b
}

// WithMetricsFactory sets an externally initialized metrics factory.
func (b *Builder) WithMetricsFactory(mf metrics.Factory) *Builder {
	b.metricsFactory = mf
	return b
}

func (b *Builder) createMainReporter(mFactory metrics.Factory, logger *zap.Logger) (*tchreporter.Reporter, error) {
	return b.CreateReporter(mFactory, logger)
}

func (b *Builder) getMetricsFactory() (metrics.Factory, error) {
	if b.metricsFactory != nil {
		return b.metricsFactory, nil
	}

	baseFactory, err := b.Metrics.CreateMetricsFactory("jaeger")
	if err != nil {
		return nil, err
	}

	return baseFactory.Namespace("agent", nil), nil
}

// CreateAgent creates the Agent
func (b *Builder) CreateAgent(logger *zap.Logger) (*Agent, error) {
	mFactory, err := b.getMetricsFactory()
	if err != nil {
		return nil, errors.Wrap(err, "cannot create metrics factory")
	}
	mainReporter, err := b.createMainReporter(mFactory, logger)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create main Reporter")
	}
	var rep reporter.Reporter = mainReporter
	if len(b.otherReporters) > 0 {
		reps := append([]reporter.Reporter{mainReporter}, b.otherReporters...)
		rep = reporter.NewMultiReporter(reps...)
	}
	processors, err := b.GetProcessors(rep, mFactory)
	if err != nil {
		return nil, err
	}
	httpServer := b.HTTPServer.GetHTTPServer(b.CollectorServiceName, mainReporter.Channel(), mFactory)
	if h := b.Metrics.Handler(); mFactory != nil && h != nil {
		httpServer.Handler.(*http.ServeMux).Handle(b.Metrics.HTTPRoute, h)
	}
	return NewAgent(processors, httpServer, logger), nil
}

// GetProcessors creates Processors with attached Reporter
func (b *Builder) GetProcessors(rep reporter.Reporter, mFactory metrics.Factory) ([]processors.Processor, error) {
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
		processor, err := cfg.GetThriftProcessor(metrics, protoFactory, handler)
		if err != nil {
			return nil, err
		}
		retMe[idx] = processor
	}
	return retMe, nil
}

// GetHTTPServer creates an HTTP server that provides sampling strategies and baggage restrictions to client libraries.
func (c HTTPServerConfiguration) GetHTTPServer(svc string, channel *tchannel.Channel, mFactory metrics.Factory) *http.Server {
	mgr := httpserver.NewCollectorProxy(svc, channel, mFactory)
	if c.HostPort == "" {
		c.HostPort = defaultHTTPServerHostPort
	}
	return httpserver.NewHTTPServer(c.HostPort, mgr, mFactory)
}

// GetThriftProcessor gets a TBufferedServer backed Processor using the collector configuration
func (c *ProcessorConfiguration) GetThriftProcessor(
	mFactory metrics.Factory,
	factory thrift.TProtocolFactory,
	handler processors.AgentProcessor,
) (processors.Processor, error) {
	c.applyDefaults()

	server, err := c.Server.getUDPServer(mFactory)
	if err != nil {
		return nil, err
	}

	return processors.NewThriftProcessor(server, c.Workers, mFactory, factory, handler)
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
