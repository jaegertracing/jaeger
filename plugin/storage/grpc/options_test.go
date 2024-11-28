// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
)

func TestOptionsWithFlags(t *testing.T) {
	v, command := config.Viperize(addFlags, tenancy.AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.server=foo:12345",
		"--multi-tenancy.header=x-scope-orgid",
	})
	require.NoError(t, err)
	var cfg Config
	require.NoError(t, initFromViper(&cfg, v))

	assert.Equal(t, "foo:12345", cfg.ClientConfig.Endpoint)
	assert.False(t, cfg.Tenancy.Enabled)
	assert.Equal(t, "x-scope-orgid", cfg.Tenancy.Header)
}

func TestRemoteOptionsWithFlags(t *testing.T) {
	v, command := config.Viperize(addFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.server=localhost:2001",
		"--grpc-storage.tls.enabled=true",
		"--grpc-storage.connection-timeout=60s",
	})
	require.NoError(t, err)
	var cfg Config
	require.NoError(t, initFromViper(&cfg, v))

	assert.Equal(t, "localhost:2001", cfg.ClientConfig.Endpoint)
	assert.False(t, cfg.ClientConfig.TLSSetting.Insecure)
	assert.Equal(t, 60*time.Second, cfg.TimeoutConfig.Timeout)
}

func TestRemoteOptionsNoTLSWithFlags(t *testing.T) {
	v, command := config.Viperize(addFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.server=localhost:2001",
		"--grpc-storage.tls.enabled=false",
		"--grpc-storage.connection-timeout=60s",
	})
	require.NoError(t, err)
	var cfg Config
	require.NoError(t, initFromViper(&cfg, v))

	assert.Equal(t, "localhost:2001", cfg.ClientConfig.Endpoint)
	assert.True(t, cfg.ClientConfig.TLSSetting.Insecure)
	assert.Equal(t, 60*time.Second, cfg.TimeoutConfig.Timeout)
}

func TestFailedTLSFlags(t *testing.T) {
	v, command := config.Viperize(addFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage.tls.enabled=false",
		"--grpc-storage.tls.cert=blah", // invalid unless tls.enabled=true
	})
	require.NoError(t, err)
	f := NewFactory(telemetry.NoopSettings())
	core, logs := observer.New(zap.NewAtomicLevelAt(zapcore.ErrorLevel))
	logger := zap.New(core, zap.WithFatalHook(zapcore.WriteThenPanic))
	require.Panics(t, func() { f.InitFromViper(v, logger) })
	require.Len(t, logs.All(), 1)
	assert.Contains(t, logs.All()[0].Message, "unable to initialize gRPC storage factory")
	assert.Contains(t, logs.All()[0].ContextMap()["error"], "failed to parse gRPC storage TLS options")
}
