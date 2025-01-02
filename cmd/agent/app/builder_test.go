// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"expvar"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	yaml "gopkg.in/yaml.v3"

	"github.com/jaegertracing/jaeger/cmd/agent/app/configmanager"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
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
      protocol: compact
      server:
        hostPort: 3.3.3.3:6831
        socketBufferSize: 16384
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
	assert.Len(t, cfg.Processors, 4)
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
		Protocol: compactProtocol,
		Workers:  10,
		Server: ServerConfiguration{
			QueueSize:        1000,
			MaxPacketSize:    65000,
			HostPort:         "3.3.3.3:6831",
			SocketBufferSize: 16384,
		},
	}, cfg.Processors[2])
	assert.Equal(t, ProcessorConfiguration{
		Model:    jaegerModel,
		Protocol: binaryProtocol,
		Workers:  20,
		Server: ServerConfiguration{
			QueueSize:     2000,
			MaxPacketSize: 65001,
			HostPort:      "3.3.3.3:6832",
		},
	}, cfg.Processors[3])
	assert.Equal(t, "4.4.4.4:5778", cfg.HTTPServer.HostPort)
}

func TestBuilderWithExtraReporter(t *testing.T) {
	cfg := &Builder{}
	agent, err := cfg.CreateAgent(fakeCollectorProxy{}, zap.NewNop(), metrics.NullFactory)
	require.NoError(t, err)
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
		{protocol: compactProtocol, model: jaegerModel, err: "no host:port provided for udp server: {QueueSize:1000 MaxPacketSize:65000 SocketBufferSize:0 HostPort:}"},
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
		require.Error(t, err)
		if testCase.err != "" {
			assert.ErrorContains(t, err, testCase.err)
		} else if testCase.errContains != "" {
			assert.ErrorContains(t, err, testCase.errContains, "error must contain %s", testCase.errContains)
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
	assert.Equal(t, rb, mr[0])
	assert.Equal(t, ra, mr[1])
}

type fakeCollectorProxy struct{}

func (fakeCollectorProxy) GetReporter() reporter.Reporter {
	return fakeCollectorProxy{}
}

func (fakeCollectorProxy) GetManager() configmanager.ClientConfigManager {
	return fakeCollectorProxy{}
}

func (fakeCollectorProxy) EmitZipkinBatch(_ context.Context, _ []*zipkincore.Span) (err error) {
	return nil
}

func (fakeCollectorProxy) EmitBatch(_ context.Context, _ *jaeger.Batch) (err error) {
	return nil
}

func (fakeCollectorProxy) Close() error {
	return nil
}

func (fakeCollectorProxy) GetSamplingStrategy(_ context.Context, _ string) (*api_v2.SamplingStrategyResponse, error) {
	return nil, errors.New("no peers available")
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
			flags: []string{"--reporter.type=grpc"},
			err:   "at least one collector hostPort address is required when resolver is not available",
		},
		{
			flags:  []string{"--reporter.type=grpc", "--reporter.grpc.host-port=foo"},
			metric: metricstest.ExpectedMetric{Name: "reporter.batches.failures", Tags: map[string]string{"protocol": "grpc", "format": "jaeger"}, Value: 1},
		},
		{
			flags:  []string{"--reporter.type=grpc", "--reporter.grpc.host-port=foo"},
			metric: metricstest.ExpectedMetric{Name: "reporter.batches.failures", Tags: map[string]string{"protocol": "grpc", "format": "jaeger"}, Value: 1},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			flags := &flag.FlagSet{}
			grpc.AddFlags(flags)
			reporter.AddFlags(flags)

			command := cobra.Command{}
			command.PersistentFlags().AddGoFlagSet(flags)
			v := viper.New()
			v.BindPFlags(command.PersistentFlags())

			err := command.ParseFlags(test.flags)
			require.NoError(t, err)

			rOpts := new(reporter.Options).InitFromViper(v, zap.NewNop())
			grpcBuilder, err := grpc.NewConnBuilder().InitFromViper(v)
			require.NoError(t, err)
			metricsFactory := metricstest.NewFactory(time.Microsecond)
			defer metricsFactory.Stop()
			builders := map[reporter.Type]CollectorProxyBuilder{
				reporter.GRPC: GRPCCollectorProxyBuilder(grpcBuilder),
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			proxy, err := CreateCollectorProxy(ctx, ProxyBuilderOptions{
				Options: *rOpts,
				Metrics: metricsFactory,
				Logger:  zap.NewNop(),
			}, builders)
			if err == nil {
				defer proxy.Close()
			}
			if test.err != "" {
				require.EqualError(t, err, test.err)
				assert.Nil(t, proxy)
			} else {
				require.NoError(t, err)
				proxy.GetReporter().EmitBatch(context.Background(), jaeger.NewBatch())
				metricsFactory.AssertCounterMetrics(t, test.metric)
			}
		})
	}
}

func TestCreateCollectorProxy_UnknownReporter(t *testing.T) {
	grpcBuilder := grpc.NewConnBuilder()
	builders := map[reporter.Type]CollectorProxyBuilder{
		reporter.GRPC: GRPCCollectorProxyBuilder(grpcBuilder),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	proxy, err := CreateCollectorProxy(ctx, ProxyBuilderOptions{}, builders)
	assert.Nil(t, proxy)
	require.EqualError(t, err, "unknown reporter type ")
}

func TestPublishOpts(t *testing.T) {
	v := viper.New()
	cfg := &Builder{}
	command := cobra.Command{}
	flags := &flag.FlagSet{}
	AddFlags(flags)
	command.PersistentFlags().AddGoFlagSet(flags)
	v.BindPFlags(command.PersistentFlags())
	err := command.ParseFlags([]string{
		"--http-server.host-port=:8080",
		"--processor.jaeger-binary.server-host-port=:1111",
		"--processor.jaeger-binary.server-max-packet-size=4242",
		"--processor.jaeger-binary.server-queue-size=24",
		"--processor.jaeger-binary.workers=42",
	})
	require.NoError(t, err)
	cfg.InitFromViper(v)

	baseMetrics := metricstest.NewFactory(time.Second)
	defer baseMetrics.Stop()
	agent, err := cfg.CreateAgent(fakeCollectorProxy{}, zap.NewNop(), baseMetrics)
	require.NoError(t, err)
	assert.NotNil(t, agent)
	require.NoError(t, agent.Run())
	defer agent.Stop()

	p := cfg.Processors[2]
	prefix := fmt.Sprintf(processorPrefixFmt, p.Model, p.Protocol)
	assert.EqualValues(t, 4242, expvar.Get(prefix+suffixServerMaxPacketSize).(*expvar.Int).Value())
	assert.EqualValues(t, 24, expvar.Get(prefix+suffixServerQueueSize).(*expvar.Int).Value())
	assert.EqualValues(t, 42, expvar.Get(prefix+suffixWorkers).(*expvar.Int).Value())
}
