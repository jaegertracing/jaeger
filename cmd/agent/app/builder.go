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
	httpReporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/http"
	tcReporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
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
	Metrics    jmetrics.Builder         `yaml:"metrics"`

	// These 3 fields are copied from tchreporter.Builder because yaml does not parse embedded structs
	CollectorHostPorts   []string `yaml:"collectorHostPorts"`
	DiscoveryMinPeers    int      `yaml:"minPeers"`
	CollectorServiceName string   `yaml:"collectorServiceName"`

	tcReporter.Builder

	// These fields are copied from http.Builder because yaml does not parse embedded structs
	Scheme    string `yaml:"scheme"`
	AuthToken string `yaml:"authToken"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`

	otherReporters []reporter.Reporter
	metricsFactory metrics.Factory
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

// WithMetricsFactory sets an externally initialized metrics factory.
func (b *Builder) WithMetricsFactory(mf metrics.Factory) *Builder {
	b.metricsFactory = mf
	return b
}

func (b *Builder) createMainReporter(mFactory metrics.Factory, logger *zap.Logger) (reporter.Reporter, error) {
	if b.useTChannelReporter() {
		logger.Info("Using TChannel to report spans to the Collector")
		return b.createTChannelMainReporter(mFactory, logger)
	}

	logger.Info("Using HTTP to report spans to the Collector")
	return b.createHTTPMainReporter(mFactory, logger)
}

func (b *Builder) createHTTPMainReporter(mFactory metrics.Factory, logger *zap.Logger) (reporter.Reporter, error) {
	hrBuilder := httpReporter.NewBuilder()

	if b.Scheme != "" {
		hrBuilder.WithScheme(b.Scheme)
	}

	if len(b.CollectorHostPorts) > 0 {
		hrBuilder.WithCollectorHostPorts(b.CollectorHostPorts)
	} else {
		return nil, fmt.Errorf(`no "CollectorHostPorts" specified`)
	}

	if b.AuthToken != "" {
		hrBuilder.WithAuthToken(b.AuthToken)
	}

	if b.Username != "" && b.Password != "" {
		hrBuilder.WithUsername(b.Username)
		hrBuilder.WithPassword(b.Password)
	}

	return hrBuilder.CreateReporter(mFactory, logger)
}

func (b *Builder) createTChannelMainReporter(mFactory metrics.Factory, logger *zap.Logger) (reporter.Reporter, error) {
	if len(b.Builder.CollectorHostPorts) == 0 {
		b.Builder.CollectorHostPorts = b.CollectorHostPorts
	}
	if b.Builder.CollectorServiceName == "" {
		b.Builder.CollectorServiceName = b.CollectorServiceName
	}
	if b.Builder.DiscoveryMinPeers == 0 {
		b.Builder.DiscoveryMinPeers = b.DiscoveryMinPeers
	}
	return b.CreateReporter(mFactory, logger)
}

func (b *Builder) getMetricsFactory() (metrics.Factory, error) {
	if b.metricsFactory != nil {
		return b.metricsFactory, nil
	}
	return b.Metrics.CreateMetricsFactory("jaeger_agent")
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
	var rep = mainReporter
	if len(b.otherReporters) > 0 {
		reps := append([]reporter.Reporter{mainReporter}, b.otherReporters...)
		rep = reporter.NewMultiReporter(reps...)
	}
	processors, err := b.GetProcessors(rep, mFactory)
	if err != nil {
		return nil, err
	}
	httpServer := b.GetHTTPServer(mainReporter, mFactory)
	if b.metricsFactory == nil {
		b.Metrics.RegisterHandler(httpServer.Handler.(*http.ServeMux))
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

func (b *Builder) useTChannelReporter() bool {
	// if we don't have credentials, we use the tchannel reporter
	// if we have an auth token or a pair of username+password, we should use the http reporter
	return b.AuthToken == "" && (b.Username == "" || b.Password == "")
}

// GetHTTPServer creates an HTTP server that provides sampling strategies and baggage restrictions to client libraries.
func (b *Builder) GetHTTPServer(r reporter.Reporter, mFactory metrics.Factory) *http.Server {
	// TODO: this manager is used for the sampling and baggage restrictions, not sure we need for this here:
	// is there a non-tchannel sampling/baggage restriction endpoint on the collector side?
	// for now, we let it be nil for non-TChannel reporters
	var mgr httpserver.ClientConfigManager

	if b.useTChannelReporter() {
		channel := r.(*tcReporter.Reporter).Channel()
		mgr = httpserver.NewCollectorProxy(b.CollectorServiceName, channel, mFactory)
	}

	if b.HTTPServer.HostPort == "" {
		b.HTTPServer.HostPort = defaultHTTPServerHostPort
	}

	return httpserver.NewHTTPServer(b.HTTPServer.HostPort, mgr, mFactory)
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
