// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"time"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/jaegertracing/jaeger/internal/tenancy"
)

type Config struct {
	configgrpc.ClientConfig `mapstructure:",squash"`
	// Writer allows overriding the endpoint for writes, e.g. to an OTLP receiver.
	// If not defined the main endpoint is used for reads and writes.
	Writer configgrpc.ClientConfig `mapstructure:"writer"`

	Tenancy                      tenancy.Options `mapstructure:"multi_tenancy"`
	exporterhelper.TimeoutConfig `mapstructure:",squash"`
}

func DefaultConfig() Config {
	return Config{
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: time.Duration(5 * time.Second),
		},
	}
}
