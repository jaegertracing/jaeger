// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package pool

import (
	"runtime"
	"time"
)

const (
	DefaultMaxConnLifetime   = time.Hour
	DefaultMaxConnIdleTime   = time.Minute * 30
	DefaultHealthCheckPeriod = time.Minute
)

type Configuration struct {
	ClientConfig ClientConfig `mapstructure:"client"`
	PoolConfig   Config       `mapstructure:"pool"`
}

type ClientConfig struct {
	Address  string `mapstructure:"address"`
	Database string `mapstructure:"database"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type Config struct {
	MaxConnLifetime   time.Duration `mapstructure:"max_connection_lifetime"`
	MaxConnIdleTime   time.Duration `mapstructure:"max_connection_idle_time"`
	MinConns          int32         `mapstructure:"min_connections"`
	MaxConns          int32         `mapstructure:"max_connections"`
	HealthCheckPeriod time.Duration `mapstructure:"health_check_period"`
}

func DefaultConfig() Configuration {
	return Configuration{
		ClientConfig{
			Address:  "127.0.0.1:9000",
			Database: "default",
			Username: "default",
			Password: "default",
		},
		Config{
			MaxConnLifetime: DefaultMaxConnLifetime,
			MaxConnIdleTime: DefaultMaxConnIdleTime,
			//nolint: gosec // G115
			MinConns: int32(runtime.NumCPU()),
			//nolint: gosec // G115
			MaxConns:          int32(runtime.NumCPU() * 2),
			HealthCheckPeriod: DefaultHealthCheckPeriod,
		},
	}
}
