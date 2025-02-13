// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package conn

// Configuration clickhouse-go client connection configuration for read trace.
// more detail see:https://clickhouse.com/docs/integrations/go#connection-settings
type Configuration struct {
	Address  []string `mapstructure:"address"`
	Database string   `mapstructure:"database"`
	Username string   `mapstructure:"username"`
	Password string   `mapstructure:"password"`
}

func DefaultConfig() Configuration {
	return Configuration{
		Address:  []string{"127.0.0.1:9000"},
		Database: "default",
		Username: "default",
		Password: "default",
	}
}
