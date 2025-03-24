// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"runtime"
	"time"
)

// Configuration all configuration of how to use clickhouse as storage.
// ClickhouseGoCfg clickhouse-go client configuration.
// ChGoConfig chpool configuration.
// CreateSchema create requre table auto if not exist.
type Configuration struct {
	ClickhouseGoCfg ClickhouseGoConfig
	ChGoConfig      ChGoConfig
	CreateSchema    bool
}

func DefaultConfiguration() Configuration {
	return Configuration{
		DefaultClickhouseGoConfig(),
		DefaultChGoConfig(),
		true,
	}
}

// ClickhouseGoConfig clickhouse-go client connection configuration for read trace.
// more detail see:https://clickhouse.com/docs/integrations/go#connection-settings
type ClickhouseGoConfig struct {
	Address  []string `mapstructure:"address"`
	Database string   `mapstructure:"database"`
	Username string   `mapstructure:"username"`
	Password string   `mapstructure:"password"`
}

func DefaultClickhouseGoConfig() ClickhouseGoConfig {
	return ClickhouseGoConfig{
		Address:  []string{"127.0.0.1:9000"},
		Database: "default",
		Username: "default",
		Password: "default",
	}
}

const (
	DefaultMaxConnLifetime   = time.Hour
	DefaultMaxConnIdleTime   = time.Minute * 30
	DefaultHealthCheckPeriod = time.Minute
)

// ChGoConfig chpool configuration for write trace.
type ChGoConfig struct {
	ClientConfig ClientConfig `mapstructure:"client"`
	PoolConfig   PoolConfig   `mapstructure:"pool"`
}

// ClientConfig client configuration be used to connect to server.
type ClientConfig struct {
	Address  string `mapstructure:"address"`
	Database string `mapstructure:"database"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// PoolConfig connection pool configuration be used to define every connection lifecycle.
type PoolConfig struct {
	MaxConnLifetime   time.Duration `mapstructure:"max_connection_lifetime"`
	MaxConnIdleTime   time.Duration `mapstructure:"max_connection_idle_time"`
	MinConns          int32         `mapstructure:"min_connections"`
	MaxConns          int32         `mapstructure:"max_connections"`
	HealthCheckPeriod time.Duration `mapstructure:"health_check_period"`
}

func DefaultChGoConfig() ChGoConfig {
	return ChGoConfig{
		ClientConfig{
			Address:  "127.0.0.1:9000",
			Database: "default",
			Username: "default",
			Password: "default",
		},
		PoolConfig{
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
