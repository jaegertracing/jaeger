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
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"

	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
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

collectorHostPorts:
    - 127.0.0.1:14267
    - 127.0.0.1:14268
    - 127.0.0.1:14269

collectorServiceName: some-collector-service
minPeers: 4
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

	assert.Equal(t, 4, cfg.DiscoveryMinPeers)
	assert.Equal(t, "some-collector-service", cfg.CollectorServiceName)
	assert.Equal(
		t,
		[]string{"127.0.0.1:14267", "127.0.0.1:14268", "127.0.0.1:14269"},
		cfg.CollectorHostPorts)
}

func TestBuilderWithExtraReporter(t *testing.T) {
	cfg := &Builder{}
	cfg.WithReporter(fakeReporter{})
	agent, err := cfg.CreateAgent(zap.NewNop())
	assert.NoError(t, err)
	assert.NotNil(t, agent)
}

func TestBuilderMetrics(t *testing.T) {
	mf := metrics.NullFactory
	b := new(Builder).WithMetricsFactory(mf)
	mf2, err := b.getMetricsFactory()
	assert.NoError(t, err)
	assert.Equal(t, mf, mf2)
}

func TestBuilderMetricsError(t *testing.T) {
	b := &Builder{}
	b.Metrics.Backend = "invalid"
	_, err := b.CreateAgent(zap.NewNop())
	assert.EqualError(t, err, "cannot create metrics factory: unknown metrics backend specified")
}

func TestBuilderWithDiscoveryError(t *testing.T) {
	cfg := &Builder{}
	cfg.WithDiscoverer(fakeDiscoverer{})
	agent, err := cfg.CreateAgent(zap.NewNop())
	assert.EqualError(t, err, "cannot create main Reporter: cannot enable service discovery: both discovery.Discoverer and discovery.Notifier must be specified")
	assert.Nil(t, agent)
}

func TestBuilderWithProcessorErrors(t *testing.T) {
	testCases := []struct {
		model       model
		protocol    protocol
		hostPort    string
		err         string
		errContains string
	}{
		{protocol: protocol("bad"), err: "cannot find protocol factory for protocol bad"},
		{protocol: compactProtocol, model: model("bad"), err: "cannot find agent processor for data model bad"},
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

type fakeReporter struct{}

func (fr fakeReporter) EmitZipkinBatch(spans []*zipkincore.Span) (err error) {
	return nil
}

func (fr fakeReporter) EmitBatch(batch *jaeger.Batch) (err error) {
	return nil
}

type fakeDiscoverer struct{}

func (fd fakeDiscoverer) Instances() ([]string, error) {
	return nil, errors.New("discoverer error")
}
