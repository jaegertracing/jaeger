// Copyright (c) 2021 The Jaeger Authors.
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

package prometheus

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/storage"
)

var _ storage.MetricsFactory = new(Factory)

func TestPrometheusFactory(t *testing.T) {
	f := NewFactory()
	assert.NoError(t, f.Initialize(zap.NewNop()))
	assert.NotNil(t, f.logger)
	assert.Equal(t, "prometheus", f.options.Primary.namespace)

	listener, err := net.Listen("tcp", "localhost:")
	assert.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	f.options.Primary.ServerURL = "http://" + listener.Addr().String()
	reader, err := f.CreateMetricsReader()

	assert.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestWithDefaultConfiguration(t *testing.T) {
	f := NewFactory()
	assert.Equal(t, f.options.Primary.ServerURL, "http://localhost:9090")
	assert.Equal(t, f.options.Primary.ConnectTimeout, 30*time.Second)
}

func TestWithConfiguration(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{
		"--prometheus.server-url=http://localhost:1234",
		"--prometheus.connect-timeout=5s",
	})
	require.NoError(t, err)

	err = f.InitFromViper(v, zap.NewNop())

	require.NoError(t, err)
	assert.Equal(t, f.options.Primary.ServerURL, "http://localhost:1234")
	assert.Equal(t, f.options.Primary.ConnectTimeout, 5*time.Second)
}

func TestFailedTLSOptions(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{
		"--prometheus.tls.enabled=false",
		"--prometheus.tls.cert=blah", // not valid unless tls.enabled=true
	})
	require.NoError(t, err)

	err = f.InitFromViper(v, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to process Prometheus TLS options")
}
