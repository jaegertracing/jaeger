// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/jaegertracing/jaeger/internal/tenancy"
)

func TestOptionsWithFlags(t *testing.T) {
	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: "foo:12345",
		},
		Tenancy: tenancy.Options{
			Enabled: false,
			Header:  "x-scope-orgid",
		},
	}

	assert.Equal(t, "foo:12345", cfg.ClientConfig.Endpoint)
	assert.False(t, cfg.Tenancy.Enabled)
	assert.Equal(t, "x-scope-orgid", cfg.Tenancy.Header)
}

func TestRemoteOptionsWithFlags(t *testing.T) {
	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: "localhost:2001",
			TLS: configtls.ClientConfig{
				Insecure: false,
			},
		},
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: 60 * time.Second,
		},
	}

	assert.Equal(t, "localhost:2001", cfg.ClientConfig.Endpoint)
	assert.False(t, cfg.ClientConfig.TLS.Insecure)
	assert.Equal(t, 60*time.Second, cfg.TimeoutConfig.Timeout)
}

func TestRemoteOptionsNoTLSWithFlags(t *testing.T) {
	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: "localhost:2001",
			TLS: configtls.ClientConfig{
				Insecure: true,
			},
		},
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: 60 * time.Second,
		},
	}

	assert.Equal(t, "localhost:2001", cfg.ClientConfig.Endpoint)
	assert.True(t, cfg.ClientConfig.TLS.Insecure)
	assert.Equal(t, 60*time.Second, cfg.TimeoutConfig.Timeout)
}
