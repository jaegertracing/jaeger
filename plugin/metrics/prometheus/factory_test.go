// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	promCfg "github.com/jaegertracing/jaeger/pkg/prometheus/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage"
)

var _ storage.MetricsFactory = new(Factory)

func TestPrometheusFactory(t *testing.T) {
	f := NewFactory()
	require.NoError(t, f.Initialize(zap.NewNop()))
	assert.NotNil(t, f.logger)

	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	f.options.ServerURL = "http://" + listener.Addr().String()
	reader, err := f.CreateMetricsReader()

	require.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestWithDefaultConfiguration(t *testing.T) {
	f := NewFactory()
	assert.Equal(t, "http://localhost:9090", f.options.ServerURL)
	assert.Equal(t, 30*time.Second, f.options.ConnectTimeout)

	assert.Equal(t, "traces_span_metrics", f.options.MetricNamespace)
	assert.Equal(t, "ms", f.options.LatencyUnit)
}

func TestWithConfiguration(t *testing.T) {
	t.Run("with custom configuration and no space in token file path", func(t *testing.T) {
		f := NewFactory()
		v, command := config.Viperize(f.AddFlags)
		err := command.ParseFlags([]string{
			"--prometheus.server-url=http://localhost:1234",
			"--prometheus.connect-timeout=5s",
			"--prometheus.token-file=test/test_file.txt",
			"--prometheus.token-override-from-context=false",
		})
		require.NoError(t, err)
		f.InitFromViper(v, zap.NewNop())
		assert.Equal(t, "http://localhost:1234", f.options.ServerURL)
		assert.Equal(t, 5*time.Second, f.options.ConnectTimeout)
		assert.Equal(t, "test/test_file.txt", f.options.TokenFilePath)
		assert.False(t, f.options.TokenOverrideFromContext)
	})
	t.Run("with space in token file path", func(t *testing.T) {
		f := NewFactory()
		v, command := config.Viperize(f.AddFlags)
		err := command.ParseFlags([]string{
			"--prometheus.token-file=test/ test file.txt",
		})
		require.NoError(t, err)
		f.InitFromViper(v, zap.NewNop())
		assert.Equal(t, "test/ test file.txt", f.options.TokenFilePath)
	})
	t.Run("with custom configuration of prometheus.query", func(t *testing.T) {
		f := NewFactory()
		v, command := config.Viperize(f.AddFlags)
		err := command.ParseFlags([]string{
			"--prometheus.query.namespace=mynamespace",
			"--prometheus.query.duration-unit=ms",
		})
		require.NoError(t, err)
		f.InitFromViper(v, zap.NewNop())
		assert.Equal(t, "mynamespace", f.options.MetricNamespace)
		assert.Equal(t, "ms", f.options.LatencyUnit)
	})
	t.Run("with invalid prometheus.query.duration-unit", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected a panic due to invalid duration-unit")
			}
		}()

		f := NewFactory()
		v, command := config.Viperize(f.AddFlags)
		err := command.ParseFlags([]string{
			"--prometheus.query.duration-unit=milliseconds",
		})
		require.NoError(t, err)
		f.InitFromViper(v, zap.NewNop())
		require.Empty(t, f.options.LatencyUnit)
	})
}

func TestFailedTLSOptions(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{
		"--prometheus.tls.enabled=false",
		"--prometheus.tls.cert=blah", // not valid unless tls.enabled=true
	})
	require.NoError(t, err)

	logger, logOut := testutils.NewLogger()

	defer func() {
		r := recover()
		t.Logf("%v", r)
		assert.Contains(t, logOut.Lines()[0], "failed to process Prometheus TLS options")
	}()

	f.InitFromViper(v, logger)
	t.Errorf("f.InitFromViper did not panic")
}

func TestEmptyFactoryConfig(t *testing.T) {
	cfg := promCfg.Configuration{}
	_, err := NewFactoryWithConfig(cfg, zap.NewNop())
	require.Error(t, err)
}

func TestFactoryConfig(t *testing.T) {
	cfg := promCfg.Configuration{
		ServerURL: "localhost:1234",
	}
	_, err := NewFactoryWithConfig(cfg, zap.NewNop())
	require.NoError(t, err)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
