// Copyright (c) 2019 The Jaeger Authors.
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
	"io"
	"net/http"
	"strconv"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/configmanager"
	"github.com/jaegertracing/jaeger/cmd/agent/app/httpserver"
	"github.com/jaegertracing/jaeger/cmd/agent/app/processors"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/jaegertracing/jaeger/ports"
	zipkinThrift "github.com/jaegertracing/jaeger/thrift-gen/agent"
)

const (
	defaultQueueSize     = 1000
	defaultMaxPacketSize = 65000
	defaultServerWorkers = 10

	jaegerModel Model = "jaeger"
	zipkinModel Model = "zipkin"

	compactProtocol Protocol = "compact"
	binaryProtocol  Protocol = "binary"
)

var defaultHTTPServerHostPort = ":" + strconv.Itoa(ports.AgentConfigServerHTTP)

// Model used to distinguish the data transfer model
type Model string

// Protocol used to distinguish the data transfer protocol
type Protocol string

var (
	protocolFactoryMap = map[Protocol]thrift.TProtocolFactory{
		compactProtocol: thrift.NewTCompactProtocolFactory(),
		binaryProtocol:  thrift.NewTBinaryProtocolFactoryDefault(),
	}
)

// CollectorProxy provides access to Reporter and ClientConfigManager
type CollectorProxy interface {
	GetReporter() reporter.Reporter
	GetManager() configmanager.ClientConfigManager
	io.Closer
}

// Builder Struct to hold configurations
type Builder struct {
	Processors []ProcessorConfiguration `yaml:"processors"`
	HTTPServer HTTPServerConfiguration  `yaml:"httpServer"`

	reporters []reporter.Reporter
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
	QueueSize        int    `yaml:"queueSize"`
	MaxPacketSize    int    `yaml:"maxPacketSize"`
	SocketBufferSize int    `yaml:"socketBufferSize"`
	HostPort         string `yaml:"hostPort" validate:"nonzero"`
}

// HTTPServerConfiguration holds config for a server providing sampling strategies and baggage restrictions to clients
type HTTPServerConfiguration struct {
	HostPort string `yaml:"hostPort" validate:"nonzero"`
}

// WithReporter adds auxiliary reporters.
func (b *Builder) WithReporter(r ...reporter.Reporter) *Builder {
	b.reporters = append(b.reporters, r...)
	return b
}

// CreateAgent creates the Agent
func (b *Builder) CreateAgent(primaryProxy CollectorProxy, logger *zap.Logger, mFactory metrics.Factory) (*Agent, error) {
	r := b.getReporter(primaryProxy)
	processors, err := b.getProcessors(r, mFactory, logger)
	if err != nil {
		return nil, fmt.Errorf("cannot create processors: %w", err)
	}
	server := b.HTTPServer.getHTTPServer(primaryProxy.GetManager(), mFactory)
	b.publishOpts(mFactory)

	return NewAgent(processors, server, logger), nil
}

func (b *Builder) getReporter(primaryProxy CollectorProxy) reporter.Reporter {
	if len(b.reporters) == 0 {
		return primaryProxy.GetReporter()
	}
	rep := make([]reporter.Reporter, len(b.reporters)+1)
	rep[0] = primaryProxy.GetReporter()
	for i, r := range b.reporters {
		rep[i+1] = r
	}
	return reporter.NewMultiReporter(rep...)
}

func (b *Builder) publishOpts(mFactory metrics.Factory) {
	internalFactory := mFactory.Namespace(metrics.NSOptions{Name: "internal"})
	for _, p := range b.Processors {
		prefix := fmt.Sprintf(processorPrefixFmt, p.Model, p.Protocol)
		internalFactory.Gauge(metrics.Options{Name: prefix + suffixServerMaxPacketSize}).
			Update(int64(p.Server.MaxPacketSize))
		internalFactory.Gauge(metrics.Options{Name: prefix + suffixServerQueueSize}).
			Update(int64(p.Server.QueueSize))
		internalFactory.Gauge(metrics.Options{Name: prefix + suffixWorkers}).
			Update(int64(p.Workers))
	}
}

func (b *Builder) getProcessors(rep reporter.Reporter, mFactory metrics.Factory, logger *zap.Logger) ([]processors.Processor, error) {
	retMe := make([]processors.Processor, len(b.Processors))
	for idx, cfg := range b.Processors {
		protoFactory, ok := protocolFactoryMap[cfg.Protocol]
		if !ok {
			return nil, fmt.Errorf("cannot find protocol factory for protocol %v", cfg.Protocol)
		}
		var handler processors.AgentProcessor
		switch cfg.Model {
		case jaegerModel, zipkinModel:
			handler = zipkinThrift.NewAgentProcessor(rep)
		default:
			return nil, fmt.Errorf("cannot find agent processor for data model %v", cfg.Model)
		}
		metrics := mFactory.Namespace(metrics.NSOptions{Name: "", Tags: map[string]string{
			"protocol": string(cfg.Protocol),
			"model":    string(cfg.Model),
		}})
		processor, err := cfg.GetThriftProcessor(metrics, protoFactory, handler, logger)
		if err != nil {
			return nil, fmt.Errorf("cannot create Thrift Processor: %w", err)
		}
		retMe[idx] = processor
	}
	return retMe, nil
}

// GetHTTPServer creates an HTTP server that provides sampling strategies and baggage restrictions to client libraries.
func (c HTTPServerConfiguration) getHTTPServer(manager configmanager.ClientConfigManager, mFactory metrics.Factory) *http.Server {
	if c.HostPort == "" {
		c.HostPort = defaultHTTPServerHostPort
	}
	return httpserver.NewHTTPServer(c.HostPort, manager, mFactory)
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
		return nil, fmt.Errorf("cannot create UDP Server: %w", err)
	}

	return processors.NewThriftProcessor(server, c.Workers, mFactory, factory, handler, logger)
}

func (c *ProcessorConfiguration) applyDefaults() {
	c.Workers = defaultInt(c.Workers, defaultServerWorkers)
}

func (c *ServerConfiguration) applyDefaults() {
	c.QueueSize = defaultInt(c.QueueSize, defaultQueueSize)
	c.MaxPacketSize = defaultInt(c.MaxPacketSize, defaultMaxPacketSize)
	c.SocketBufferSize = defaultInt(c.SocketBufferSize, 0)
}

// getUDPServer gets a TBufferedServer backed server using the server configuration
func (c *ServerConfiguration) getUDPServer(mFactory metrics.Factory) (servers.Server, error) {
	c.applyDefaults()

	if c.HostPort == "" {
		return nil, fmt.Errorf("no host:port provided for udp server: %+v", *c)
	}
	transport, err := thriftudp.NewTUDPServerTransport(c.HostPort)
	if err != nil {
		return nil, fmt.Errorf("cannot create UDPServerTransport: %w", err)
	}
	if c.SocketBufferSize != 0 {
		if err := transport.SetSocketBufferSize(c.SocketBufferSize); err != nil {
			return nil, fmt.Errorf("cannot set UDP socket buffer size: %w", err)
		}
	}

	return servers.NewTBufferedServer(transport, c.QueueSize, c.MaxPacketSize, mFactory)
}

func defaultInt(value int, defaultVal int) int {
	if value == 0 {
		value = defaultVal
	}
	return value
}

// ProxyBuilderOptions holds config for CollectorProxyBuilder
type ProxyBuilderOptions struct {
	reporter.Options
	Logger  *zap.Logger
	Metrics metrics.Factory
}

// CollectorProxyBuilder is a func which builds CollectorProxy.
type CollectorProxyBuilder func(ProxyBuilderOptions) (CollectorProxy, error)

// CreateCollectorProxy creates collector proxy
func CreateCollectorProxy(
	opts ProxyBuilderOptions,
	builders map[reporter.Type]CollectorProxyBuilder,
) (CollectorProxy, error) {
	builder, ok := builders[opts.ReporterType]
	if !ok {
		return nil, fmt.Errorf("unknown reporter type %s", string(opts.ReporterType))
	}
	return builder(opts)
}
