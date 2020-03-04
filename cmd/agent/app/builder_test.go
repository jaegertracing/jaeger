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
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"
	yaml "gopkg.in/yaml.v2"

	"github.com/jaegertracing/jaeger/cmd/agent/app/configmanager"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	"github.com/jaegertracing/jaeger/tchannel/agent/app/reporter/tchannel"
	"github.com/jaegertracing/jaeger/tchannel/collector/app"
	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
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
	agent, err := cfg.CreateAgent(fakeCollectorProxy{}, zap.NewNop(), metrics.NullFactory)
	assert.NoError(t, err)
	assert.NotNil(t, agent)
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
		_, err := cfg.CreateAgent(&fakeCollectorProxy{}, zap.NewNop(), metrics.NullFactory)
		assert.Error(t, err)
		if testCase.err != "" {
			assert.Contains(t, err.Error(), testCase.err)
		} else if testCase.errContains != "" {
			assert.True(t, strings.Contains(err.Error(), testCase.errContains), "error must contain %s", testCase.errContains)
		}
	}
}

func TestMultipleCollectorProxies(t *testing.T) {
	b := Builder{}
	ra := fakeCollectorProxy{}
	rb := fakeCollectorProxy{}
	b.WithReporter(ra)
	r := b.getReporter(rb)
	mr, ok := r.(reporter.MultiReporter)
	require.True(t, ok)
	fmt.Println(mr)
	assert.Equal(t, rb, mr[0])
	assert.Equal(t, ra, mr[1])
}

type fakeCollectorProxy struct {
}

func (f fakeCollectorProxy) GetReporter() reporter.Reporter {
	return fakeCollectorProxy{}
}
func (f fakeCollectorProxy) GetManager() configmanager.ClientConfigManager {
	return fakeCollectorProxy{}
}

func (fakeCollectorProxy) EmitZipkinBatch(spans []*zipkincore.Span) (err error) {
	return nil
}
func (fakeCollectorProxy) EmitBatch(batch *jaeger.Batch) (err error) {
	return nil
}
func (fakeCollectorProxy) Close() error {
	return nil
}

func (f fakeCollectorProxy) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return nil, errors.New("no peers available")
}
func (fakeCollectorProxy) GetBaggageRestrictions(serviceName string) ([]*baggage.BaggageRestriction, error) {
	return nil, nil
}

func TestCreateCollectorProxy(t *testing.T) {
	tests := []struct {
		flags  []string
		err    string
		metric metricstest.ExpectedMetric
	}{
		{
			err: "at least one collector hostPort address is required when resolver is not available",
		},
		{
			flags: []string{"--collector.host-port=foo"},
			err:   "at least one collector hostPort address is required when resolver is not available",
		},
		{
			flags:  []string{"--reporter.type=tchannel"},
			metric: metricstest.ExpectedMetric{Name: "reporter.batches.failures", Tags: map[string]string{"protocol": "tchannel", "format": "jaeger"}, Value: 1},
		},
		{
			flags:  []string{"--reporter.type=tchannel", "--collector.host-port=foo"},
			metric: metricstest.ExpectedMetric{Name: "reporter.batches.failures", Tags: map[string]string{"protocol": "tchannel", "format": "jaeger"}, Value: 1},
		},
		{
			flags: []string{"--reporter.type=grpc", "--collector.host-port=foo"},
			err:   "at least one collector hostPort address is required when resolver is not available",
		},
		{
			flags:  []string{"--reporter.type=grpc", "--reporter.grpc.host-port=foo", "--collector.host-port=foo"},
			metric: metricstest.ExpectedMetric{Name: "reporter.batches.failures", Tags: map[string]string{"protocol": "grpc", "format": "jaeger"}, Value: 1},
		},
		{
			flags:  []string{"--reporter.type=grpc", "--reporter.grpc.host-port=foo"},
			metric: metricstest.ExpectedMetric{Name: "reporter.batches.failures", Tags: map[string]string{"protocol": "grpc", "format": "jaeger"}, Value: 1},
		},
	}

	for _, test := range tests {
		flags := &flag.FlagSet{}
		tchannel.AddFlags(flags)
		grpc.AddFlags(flags)
		new(reporter.Flags).AddFlags(flags)
		app.AddFlags(flags)

		command := cobra.Command{}
		command.PersistentFlags().AddGoFlagSet(flags)
		v := viper.New()
		v.BindPFlags(command.PersistentFlags())

		err := command.ParseFlags(test.flags)
		require.NoError(t, err)

		rOpts := new(reporter.Options).InitFromViper(v, zap.NewNop())
		tchan := tchannel.NewBuilder().InitFromViper(v, zap.NewNop())
		grpcBuilder := grpc.NewConnBuilder().InitFromViper(v)

		metricsFactory := metricstest.NewFactory(time.Microsecond)

		builders := map[reporter.Type]CollectorProxyBuilder{}
		builders[reporter.GRPC] = GRPCCollectorProxyBuilder(grpcBuilder)
		builders[tchannel.ReporterType] = TCollectorProxyBuilder(tchan)

		proxy, err := CreateCollectorProxy(ProxyBuilderOptions{
			Options: *rOpts,
			Metrics: metricsFactory,
			Logger:  zap.NewNop(),
		}, builders)
		if test.err != "" {
			assert.EqualError(t, err, test.err)
			assert.Nil(t, proxy)
		} else {
			require.NoError(t, err)
			proxy.GetReporter().EmitBatch(jaeger.NewBatch())
			metricsFactory.AssertCounterMetrics(t, test.metric)
		}
	}
}

func TestCreateCollectorProxy_UnknownReporter(t *testing.T) {
	tchan := tchannel.NewBuilder()
	grpcBuilder := grpc.NewConnBuilder()

	builders := map[reporter.Type]CollectorProxyBuilder{}
	builders[reporter.GRPC] = GRPCCollectorProxyBuilder(grpcBuilder)
	builders[tchannel.ReporterType] = TCollectorProxyBuilder(tchan)

	proxy, err := CreateCollectorProxy(ProxyBuilderOptions{}, builders)
	assert.Nil(t, proxy)
	assert.EqualError(t, err, "unknown reporter type ")
}
