// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"go.opentelemetry.io/collector/config/configtls"
)

const (
	defaultServer   = "localhost:9090"
	defaultDatabase = "jaeger"
	defaultUsername = "default"
	defaultPassword = "default"
)

// Config holds all configuration options for using ClickHouse as storage.
// more detail see:https://clickhouse.com/docs/integrations/go#connection-settings
type Config struct {
	// Servers is a slice of addresses including the port.
	// (i.e. 127.0.0.1:9000, 127.0.0.1:9001)
	Servers  []string       `mapstructure:"servers"`
	Database string         `mapstructure:"database"`
	Auth     Authentication `mapstructure:"auth"`
	// TLS contains the TLS configuration for the connection to Clickhouse servers.
	TLS configtls.ClientConfig `mapstructure:"tls"`
}

type Authentication struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

func DefaultConfiguration() Config {
	return Config{
		Servers:  []string{defaultServer},
		Database: defaultDatabase,
		Auth: Authentication{
			Username: defaultUsername,
			Password: defaultPassword,
		},
		TLS: configtls.NewDefaultClientConfig(),
	}
}
