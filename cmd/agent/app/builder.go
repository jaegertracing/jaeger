// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package app

import (
	"fmt"
	"net/http"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/tchannel-go"
	"go.uber.org/zap"

	"github.com/uber/jaeger/cmd/agent/app/httpserver"
	"github.com/uber/jaeger/cmd/agent/app/processors"
	"github.com/uber/jaeger/cmd/agent/app/reporter"

	tchreporter "github.com/uber/jaeger/cmd/agent/app/reporter/tchannel"

	"github.com/uber/jaeger/cmd/agent/app/servers"
	"github.com/uber/jaeger/cmd/agent/app/servers/thriftudp"
	zipkinThrift "github.com/uber/jaeger/thrift-gen/agent"
	jaegerThrift "github.com/uber/jaeger/thrift-gen/jaeger"
)

const (
	defaultQueueSize     = 1000
	defaultMaxPacketSize = 65000
	defaultServerWorkers = 10
	defaultMinPeers      = 3

	defaultHTTPServerHostPort = ":5778"

	agentServiceName            = "jaeger-agent"
	defaultCollectorServiceName = "jaeger-collector"

	jaegerModel model = "jaeger"
	zipkinModel       = "zipkin"

	compactProtocol protocol = "compact"
	binaryProtocol           = "binary"
)

type model string
type protocol string

var (
	errNoReporters = errors.New("agent requires at least one Reporter")

	protocolFactoryMap = map[protocol]thrift.TProtocolFactory{
		compactProtocol: thrift.NewTCompactProtocolFactory(),
		binaryProtocol:  thrift.NewTBinaryProtocolFactoryDefault(),
	}
)

// Builder Struct to hold configurations
type Builder struct {
	Processors []ProcessorConfiguration `yaml:"processors"`
	HTTPServer HTTPServerConfiguration  `yaml:"httpServer"`

	// These 3 fields are copied from reporter.Builder because yaml does not parse embedded structs
	CollectorHostPorts   []string `yaml:"collectorHostPorts"`
	DiscoveryMinPeers    int      `yaml:"minPeers"`
	CollectorServiceName string   `yaml:"collectorServiceName"`

	tchreporter.Builder

	otherReporters []reporter.Reporter
}

// NewBuilder creates a default builder with three processors.
func NewBuilder() *Builder {
	return &Builder{
		Processors: []ProcessorConfiguration{
			{
				Workers:  defaultServerWorkers,
				Model:    zipkinModel,
				Protocol: compactProtocol,
				Server: ServerConfiguration{
					QueueSize:     defaultQueueSize,
					MaxPacketSize: defaultMaxPacketSize,
					HostPort:      ":5775",
				},
			},
			{
				Workers:  defaultServerWorkers,
				Model:    jaegerModel,
				Protocol: compactProtocol,
				Server: ServerConfiguration{
					QueueSize:     defaultQueueSize,
					MaxPacketSize: defaultMaxPacketSize,
					HostPort:      ":6831",
				},
			},
			{
				Workers:  defaultServerWorkers,
				Model:    jaegerModel,
				Protocol: binaryProtocol,
				Server: ServerConfiguration{
					QueueSize:     defaultQueueSize,
					MaxPacketSize: defaultMaxPacketSize,
					HostPort:      ":6832",
				},
			},
		},
		HTTPServer: HTTPServerConfiguration{
			HostPort: ":5778",
		},
	}
}

// ProcessorConfiguration holds config for a processor that receives spans from Server
type ProcessorConfiguration struct {
	Workers  int                 `yaml:"workers"`
	Model    model               `yaml:"model"`
	Protocol protocol            `yaml:"protocol"`
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

func (b *Builder) createMainReporter(mFactory metrics.Factory, logger *zap.Logger) (*tchreporter.Reporter, error) {
	if len(b.Builder.CollectorHostPorts) == 0 {
		b.Builder.CollectorHostPorts = b.CollectorHostPorts
	}
	if b.Builder.CollectorServiceName == "" {
		b.Builder.CollectorServiceName = b.CollectorServiceName
	}
	if b.Builder.DiscoveryMinPeers == 0 {
		b.Builder.DiscoveryMinPeers = b.DiscoveryMinPeers
	}
	return b.Builder.CreateReporter(mFactory, logger)
}

// CreateAgent creates the Agent
func (b *Builder) CreateAgent(mFactory metrics.Factory, logger *zap.Logger) (*Agent, error) {
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
	return NewAgent(processors, httpServer, nil, logger), nil
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
