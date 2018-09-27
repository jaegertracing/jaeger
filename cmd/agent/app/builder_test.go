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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"

	"github.com/jaegertracing/jaeger/cmd/agent/app/httpserver"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

var yamlConfig = `
ignored: abcd

processors:
    - model: zipkin
      protocol: compact
      server:
        hostPort: 1.1.1.1:5775
    - model: jaeger
      protocol: compact
      server:
        hostPort: 2.2.2.2:6831
    - model: jaeger
      protocol: binary
      workers: 20
      server:
        queueSize: 2000
        maxPacketSize: 65001
        hostPort: 3.3.3.3:6832

httpServer:
    hostPort: 4.4.4.4:5778
`

func TestBuilderFromConfig(t *testing.T) {
	cfg := Builder{}
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	require.NoError(t, err)
	assert.Len(t, cfg.Processors, 3)
	for i := range cfg.Processors {
		cfg.Processors[i].applyDefaults()
		cfg.Processors[i].Server.applyDefaults()
	}
	assert.Equal(t, ProcessorConfiguration{
		Model:    zipkinModel,
		Protocol: compactProtocol,
		Workers:  10,
		Server: ServerConfiguration{
			QueueSize:     1000,
			MaxPacketSize: 65000,
			HostPort:      "1.1.1.1:5775",
		},
	}, cfg.Processors[0])
	assert.Equal(t, ProcessorConfiguration{
		Model:    jaegerModel,
		Protocol: compactProtocol,
		Workers:  10,
		Server: ServerConfiguration{
			QueueSize:     1000,
			MaxPacketSize: 65000,
			HostPort:      "2.2.2.2:6831",
		},
	}, cfg.Processors[1])
	assert.Equal(t, ProcessorConfiguration{
		Model:    jaegerModel,
		Protocol: binaryProtocol,
		Workers:  20,
		Server: ServerConfiguration{
			QueueSize:     2000,
			MaxPacketSize: 65001,
			HostPort:      "3.3.3.3:6832",
		},
	}, cfg.Processors[2])
	assert.Equal(t, "4.4.4.4:5778", cfg.HTTPServer.HostPort)
}

func TestBuilderWithExtraReporter(t *testing.T) {
	cfg := &Builder{}
	configureSamplingManager(t, cfg)
	cfg.WithReporters(fakeReporter{})
	agent, err := cfg.CreateAgent(zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, agent)
}

func TestBuilderMetrics(t *testing.T) {
	mf := metrics.NullFactory
	b := new(Builder).WithMetricsFactory(mf)
	mf2, err := b.GetMetricsFactory()
	assert.NoError(t, err)
	assert.Equal(t, mf, mf2)
}

func TestBuilderMetricsHandler(t *testing.T) {
	b := &Builder{}
	configureSamplingManager(t, b)
	b.Metrics.Backend = "expvar"
	b.Metrics.HTTPRoute = "/expvar"
	factory, err := b.Metrics.CreateMetricsFactory("test")
	assert.NoError(t, err)
	assert.NotNil(t, factory)
	b.metricsFactory = factory
	agent, err := b.CreateAgent(zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, agent)
}

func TestBuilderMetricsError(t *testing.T) {
	b := &Builder{}
	b.Metrics.Backend = "invalid"
	_, err := b.CreateAgent(zap.NewNop())
	assert.EqualError(t, err, "cannot create metrics factory: unknown metrics backend specified")
}

func TestBuilderWithProcessorErrors(t *testing.T) {
	testCases := []struct {
		model       Model
		protocol    Protocol
		hostPort    string
		err         string
		errContains string
	}{
		{protocol: Protocol("bad"), err: "cannot find protocol factory for protocol bad"},
		{protocol: compactProtocol, model: Model("bad"), err: "cannot find agent processor for data model bad"},
		{protocol: compactProtocol, model: jaegerModel, err: "no host:port provided for udp server: {QueueSize:1000 MaxPacketSize:65000 HostPort:}"},
		{protocol: compactProtocol, model: zipkinModel, hostPort: "bad-host-port", errContains: "bad-host-port"},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		cfg := &Builder{
			Processors: []ProcessorConfiguration{
				{
					Model:    testCase.model,
					Protocol: testCase.protocol,
					Server: ServerConfiguration{
						HostPort: testCase.hostPort,
					},
				},
			},
		}
		_, err := cfg.CreateAgent(zap.NewNop())
		assert.Error(t, err)
		if testCase.err != "" {
			assert.EqualError(t, err, testCase.err)
		} else if testCase.errContains != "" {
			assert.True(t, strings.Contains(err.Error(), testCase.errContains), "error must contain %s", testCase.errContains)
		}
	}
}

func configureSamplingManager(t *testing.T, cfg *Builder) {
	m, err := cfg.GetMetricsFactory()
	require.NoError(t, err)
	r, err := tchannel.NewBuilder().CreateReporter(m, zap.NewNop())
	require.NoError(t, err)
	cfg.WithReporters(r).WithClientConfigManager(httpserver.NewCollectorProxy(r.CollectorServiceName(), r.Channel(), m))
}

type fakeReporter struct{}

func (fr fakeReporter) EmitZipkinBatch(spans []*zipkincore.Span) (err error) {
	return nil
}

func (fr fakeReporter) EmitBatch(batch *jaeger.Batch) (err error) {
	return nil
}
