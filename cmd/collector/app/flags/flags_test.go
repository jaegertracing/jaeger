// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/testutils"
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

	assert.Equal(t, ":5678", c.HTTP.Endpoint)
	assert.Equal(t, ":1234", c.GRPC.NetAddr.Endpoint)
	assert.Equal(t, ":3456", c.Zipkin.Endpoint)
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

	assert.Equal(t, ":5678", c.HTTP.Endpoint)
	assert.Equal(t, "127.0.0.1:1234", c.GRPC.NetAddr.Endpoint)
	assert.Equal(t, "0.0.0.0:3456", c.Zipkin.Endpoint)
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
			assert.ErrorContains(t, err, "failed to parse")
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

	assert.Equal(t, 8, c.GRPC.MaxRecvMsgSizeMiB)
}

func TestCollectorOptionsWithFlags_CheckMaxConnectionAge(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--collector.grpc-server.max-connection-age=5m",
		"--collector.grpc-server.max-connection-age-grace=1m",
		"--collector.http-server.idle-timeout=5m",
		"--collector.http-server.read-timeout=6m",
		"--collector.http-server.read-header-timeout=5s",
	})
	_, err := c.InitFromViper(v, zap.NewNop())
	require.NoError(t, err)

	assert.Equal(t, 5*time.Minute, c.GRPC.Keepalive.ServerParameters.MaxConnectionAge)
	assert.Equal(t, time.Minute, c.GRPC.Keepalive.ServerParameters.MaxConnectionAgeGrace)
	assert.Equal(t, 5*time.Minute, c.HTTP.IdleTimeout)
	assert.Equal(t, 6*time.Minute, c.HTTP.ReadTimeout)
	assert.Equal(t, 5*time.Second, c.HTTP.ReadHeaderTimeout)
}

func TestCollectorOptionsWithFlags_CheckNoTenancy(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{})
	c.InitFromViper(v, zap.NewNop())

	assert.False(t, c.Tenancy.Enabled)
}

func TestCollectorOptionsWithFlags_CheckSimpleTenancy(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--multi-tenancy.enabled=true",
	})
	c.InitFromViper(v, zap.NewNop())

	assert.True(t, c.Tenancy.Enabled)
	assert.Equal(t, "x-tenant", c.Tenancy.Header)
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

	assert.True(t, c.Tenancy.Enabled)
	assert.Equal(t, "custom-tenant-header", c.Tenancy.Header)
	assert.Equal(t, []string{"acme", "hardware-store"}, c.Tenancy.Tenants)
}

func TestCollectorOptionsWithFlags_CheckZipkinKeepAlive(t *testing.T) {
	c := &CollectorOptions{}
	v, command := config.Viperize(AddFlags)
	command.ParseFlags([]string{
		"--collector.zipkin.keep-alive=false",
	})
	c.InitFromViper(v, zap.NewNop())

	assert.False(t, c.Zipkin.KeepAlive)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
