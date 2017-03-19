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
	"github.com/uber-go/zap"
	"github.com/uber/tchannel-go"
	tchannelThrift "github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger-lib/metrics"
	zipkinThrift "github.com/uber/jaeger/thrift-gen/agent"
	jaegerThrift "github.com/uber/jaeger/thrift-gen/jaeger"

	"github.com/uber/jaeger/cmd/agent/app/processors"
	"github.com/uber/jaeger/cmd/agent/app/reporter"
	"github.com/uber/jaeger/cmd/agent/app/sampling"
	"github.com/uber/jaeger/cmd/agent/app/servers"
	"github.com/uber/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/uber/jaeger/pkg/discovery"
	"github.com/uber/jaeger/pkg/discovery/peerlistmgr"
)

const (
	defaultQueueSize     = 1000
	defaultMaxPacketSize = 65000
	defaultServerWorkers = 10
	defaultMinPeers      = 3

	defaultSamplingServerHostPort = "localhost:5778"

	agentServiceName     = "jaeger-agent"
	collectorServiceName = "tcollector" // for legacy reasons

	jaegerModel model = "jaeger"
	zipkinModel       = "zipkin"

	compactProtocol protocol = "compact"
	binaryProtocol           = "binary"
)

type model string
type protocol string

var (
	protocolFactoryMap = map[protocol]thrift.TProtocolFactory{
		compactProtocol: thrift.NewTCompactProtocolFactory(),
		binaryProtocol:  thrift.NewTBinaryProtocolFactoryDefault(),
	}
)

// Builder Struct to hold configurations
type Builder struct {
	Processors     []ProcessorConfiguration    `yaml:"processors"`
	SamplingServer SamplingServerConfiguration `yaml:"samplingServer"`

	// CollectorHostPort is the hostPort for Jaeger Collector.
	// Set this to communicate with Collector directly, without routing layer.
	CollectorHostPort string `yaml:"collectorHostPort"`

	// MinPeers is the min number of servers we want the agent to connect to.
	// If zero, defaults to min(3, number of peers returned by service discovery)
	DiscoveryMinPeers int `yaml:"minPeers"`

	discoverer     discovery.Discoverer
	notifier       discovery.Notifier
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
					HostPort:      "127.0.0.1:5775",
				},
			},
			{
				Workers:  defaultServerWorkers,
				Model:    jaegerModel,
				Protocol: compactProtocol,
				Server: ServerConfiguration{
					QueueSize:     defaultQueueSize,
					MaxPacketSize: defaultMaxPacketSize,
					HostPort:      "127.0.0.1:6831",
				},
			},
			{
				Workers:  defaultServerWorkers,
				Model:    jaegerModel,
				Protocol: binaryProtocol,
				Server: ServerConfiguration{
					QueueSize:     defaultQueueSize,
					MaxPacketSize: defaultMaxPacketSize,
					HostPort:      "127.0.0.1:6832",
				},
			},
		},
		SamplingServer: SamplingServerConfiguration{
			HostPort: "127.0.0.1:5778",
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

// SamplingServerConfiguration holds config for a server providing sampling strategies to clients
type SamplingServerConfiguration struct {
	HostPort string `yaml:"hostPort" validate:"nonzero"`
}

// WithReporter adds auxilary reporters
func (b *Builder) WithReporter(r reporter.Reporter) *Builder {
	b.otherReporters = append(b.otherReporters, r)
	return b
}

// WithDiscoverer sets service discovery
func (b *Builder) WithDiscoverer(d discovery.Discoverer) *Builder {
	b.discoverer = d
	return b
}

// WithDiscoveryNotifier sets service discovery notifier
func (b *Builder) WithDiscoveryNotifier(n discovery.Notifier) *Builder {
	b.notifier = n
	return b
}

func (b *Builder) enableDiscovery(channel *tchannel.Channel, logger zap.Logger) (interface{}, error) {
	if b.discoverer == nil && b.notifier == nil {
		return nil, nil
	}
	if b.discoverer == nil || b.notifier == nil {
		return nil, errors.New("both discovery.Discoverer and discovery.Notifier must be specified")
	}

	logger.Info("Enabling service discovery", zap.String("service", collectorServiceName))

	subCh := channel.GetSubChannel(collectorServiceName, tchannel.Isolated)
	peers := subCh.Peers()
	return peerlistmgr.New(peers, b.discoverer, b.notifier,
		peerlistmgr.Options.MinPeers(defaultInt(b.DiscoveryMinPeers, defaultMinPeers)),
		peerlistmgr.Options.Logger(logger))
}

// CreateAgent creates the Agent
func (b *Builder) CreateAgent(mFactory metrics.Factory, logger zap.Logger) (*Agent, error) {
	// ignore errors since it only happens on empty service name
	channel, _ := tchannel.NewChannel(agentServiceName, nil)

	discoveryMgr, err := b.enableDiscovery(channel, logger)
	if err != nil {
		return nil, errors.Wrap(err, "cannot enable service discovery")
	}
	var clientOpts *tchannelThrift.ClientOptions
	if discoveryMgr == nil && b.CollectorHostPort != "" {
		clientOpts = &tchannelThrift.ClientOptions{HostPort: b.CollectorHostPort}
	}
	rep := reporter.NewTCollectorReporter(channel, mFactory, logger, clientOpts)
	if b.otherReporters != nil {
		reps := append([]reporter.Reporter{}, b.otherReporters...)
		reps = append(reps, rep)
		rep = reporter.NewMultiReporter(reps...)
	}
	processors, err := b.GetProcessors(rep, mFactory)
	if err != nil {
		return nil, err
	}
	samplingServer := b.SamplingServer.GetSamplingServer(channel, mFactory, clientOpts)
	return NewAgent(processors, samplingServer, discoveryMgr, logger), nil
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

// GetSamplingServer creates an HTTP server that provides sampling strategies to client libraries.
func (c SamplingServerConfiguration) GetSamplingServer(channel *tchannel.Channel, mFactory metrics.Factory, clientOpts *tchannelThrift.ClientOptions) *http.Server {
	samplingMgr := sampling.NewTCollectorSamplingManagerProxy(channel, mFactory, clientOpts)
	if c.HostPort == "" {
		c.HostPort = defaultSamplingServerHostPort
	}
	return sampling.NewSamplingServer(c.HostPort, samplingMgr, mFactory)
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
