// Copyright (c) 2020 The Jaeger Authors.
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

package flags

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestCollectorOptionsWithFlags_CheckHostPort(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--collector.http-server.host-port=5678",
		"--collector.grpc-server.host-port=1234",
		"--collector.zipkin.host-port=3456",
	})
	_, err := c.InitFromViper(v, zap.NewNop())
	require.NoError(t, err)

	assert.Equal(t, ":5678", c.HTTP.HostPort)
	assert.Equal(t, ":1234", c.GRPC.HostPort)
	assert.Equal(t, ":3456", c.Zipkin.HTTPHostPort)
}

func TestCollectorOptionsWithFlags_CheckFullHostPort(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--collector.http-server.host-port=:5678",
		"--collector.grpc-server.host-port=127.0.0.1:1234",
		"--collector.zipkin.host-port=0.0.0.0:3456",
	})
	_, err := c.InitFromViper(v, zap.NewNop())
	require.NoError(t, err)

	assert.Equal(t, ":5678", c.HTTP.HostPort)
	assert.Equal(t, "127.0.0.1:1234", c.GRPC.HostPort)
	assert.Equal(t, "0.0.0.0:3456", c.Zipkin.HTTPHostPort)
}

func TestCollectorOptionsWithFailedTLSFlags(t *testing.T) {
	prefixes := []string{
		"--collector.http",
		"--collector.grpc",
		"--collector.zipkin",
		"--collector.otlp.http",
		"--collector.otlp.grpc",
	}
	for _, prefix := range prefixes {
		t.Run(prefix, func(t *testing.T) {
			c := &CollectorOptions{}
			v, command := config.Viperize(AddFlags)
			err := command.ParseFlags([]string{
				prefix + ".tls.enabled=false",
				prefix + ".tls.cert=blah", // invalid unless tls.enabled
			})
			require.NoError(t, err)
			_, err = c.InitFromViper(v, zap.NewNop())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to parse")
		})
	}
}

func TestCollectorOptionsWithFlags_CheckMaxReceiveMessageLength(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--collector.grpc-server.max-message-size=8388608",
	})
	_, err := c.InitFromViper(v, zap.NewNop())
	require.NoError(t, err)

	assert.Equal(t, 8388608, c.GRPC.MaxReceiveMessageLength)
}

func TestCollectorOptionsWithFlags_CheckMaxConnectionAge(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--collector.grpc-server.max-connection-age=5m",
		"--collector.grpc-server.max-connection-age-grace=1m",
	})
	_, err := c.InitFromViper(v, zap.NewNop())
	require.NoError(t, err)

	assert.Equal(t, 5*time.Minute, c.GRPC.MaxConnectionAge)
	assert.Equal(t, time.Minute, c.GRPC.MaxConnectionAgeGrace)
}

func TestCollectorOptionsWithFlags_CheckNoTenancy(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{})
	c.InitFromViper(v, zap.NewNop())

	assert.Equal(t, false, c.GRPC.Tenancy.Enabled)
}

func TestCollectorOptionsWithFlags_CheckSimpleTenancy(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--multi-tenancy.enabled=true",
	})
	c.InitFromViper(v, zap.NewNop())

	assert.Equal(t, true, c.GRPC.Tenancy.Enabled)
	assert.Equal(t, "x-tenant", c.GRPC.Tenancy.Header)
}

func TestCollectorOptionsWithFlags_CheckFullTenancy(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--multi-tenancy.enabled=true",
		"--multi-tenancy.header=custom-tenant-header",
		"--multi-tenancy.tenants=acme,hardware-store",
	})
	c.InitFromViper(v, zap.NewNop())

	assert.Equal(t, true, c.GRPC.Tenancy.Enabled)
	assert.Equal(t, "custom-tenant-header", c.GRPC.Tenancy.Header)
	assert.Equal(t, []string{"acme", "hardware-store"}, c.GRPC.Tenancy.Tenants)
}
