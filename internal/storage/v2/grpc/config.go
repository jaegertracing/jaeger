// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"time"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/jaegertracing/jaeger/internal/tenancy"
)

const (
	defaultConnectionTimeout = time.Duration(5 * time.Second)
)

type Config struct {
	Tenancy                      tenancy.Options `mapstructure:"multi_tenancy"`
	configgrpc.ClientConfig      `mapstructure:",squash"`
	exporterhelper.TimeoutConfig `mapstructure:",squash"`
}

func DefaultConfig() Config {
	return Config{
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: defaultConnectionTimeout,
		},
	}
}
