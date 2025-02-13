// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"runtime"
	"time"

	"github.com/ClickHouse/ch-go"
	"github.com/ClickHouse/ch-go/chpool"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/clickhouse"
	"github.com/jaegertracing/jaeger/pkg/clickhouse/wrapper"
)

const (
	DefaultMaxConnLifetime   = time.Hour
	DefaultMaxConnIdleTime   = time.Minute * 30
	DefaultHealthCheckPeriod = time.Minute
)

// Configuration describes the configuration properties needed to connect to an ElasticSearch cluster
type Configuration struct {
	ClientConfig         ClientConfig         `mapstructure:"client"`
	ConnectionPoolConfig ConnectionPoolConfig `mapstructure:"pool"`
}

type ClientConfig struct {
	Address  string `mapstructure:"address"`
	Database string `mapstructure:"database"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type ConnectionPoolConfig struct {
	MaxConnLifetime   time.Duration `mapstructure:"max_connection_lifetime"`
	MaxConnIdleTime   time.Duration `mapstructure:"max_connection_idle_time"`
	MinConns          int32         `mapstructure:"min_connections"`
	MaxConns          int32         `mapstructure:"max_connections"`
	HealthCheckPeriod time.Duration `mapstructure:"health_check_period"`
}

func DefaultConfiguration() Configuration {
	return Configuration{
		ClientConfig: ClientConfig{
			Address:  "127.0.0.1:9000",
			Database: "jaeger",
			Username: "default",
			Password: "default",
		},
		ConnectionPoolConfig: ConnectionPoolConfig{
			MaxConnLifetime: DefaultMaxConnLifetime,
			MaxConnIdleTime: DefaultMaxConnIdleTime,
			//nolint: gosec // G115
			MaxConns: int32(runtime.NumCPU() * 2),
			//nolint: gosec // G115
			MinConns:          int32(runtime.NumCPU()),
			HealthCheckPeriod: DefaultHealthCheckPeriod,
		},
	}
}

func (c *Configuration) NewClient(logger *zap.Logger) (clickhouse.Client, error) {
	pool, err := c.newPool(logger)
	if err != nil {
		return nil, err
	}
	return wrapper.WrapCHClient(nil, pool), nil
}

func (c *Configuration) newPool(log *zap.Logger) (*chpool.Pool, error) {
	option := chpool.Options{
		ClientOptions: ch.Options{
			Logger:   log,
			Address:  c.ClientConfig.Address,
			Database: c.ClientConfig.Database,
			User:     c.ClientConfig.Username,
			Password: c.ClientConfig.Password,
		},
		MaxConnLifetime:   c.ConnectionPoolConfig.MaxConnLifetime,
		MaxConnIdleTime:   c.ConnectionPoolConfig.MaxConnIdleTime,
		MaxConns:          c.ConnectionPoolConfig.MaxConns,
		MinConns:          c.ConnectionPoolConfig.MinConns,
		HealthCheckPeriod: c.ConnectionPoolConfig.HealthCheckPeriod,
	}
	chPool, err := chpool.Dial(context.Background(), option)
	return chPool, err
}
