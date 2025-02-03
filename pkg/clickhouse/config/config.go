// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

// Configuration is clickhouse's internal configuration data
type Configuration struct {
	Connection Connection `mapstructure:"connection"`
}

type Connection struct {
	// Servers contains a list of hosts that are used to connect to the cluster.
	Servers []string `mapstructure:"servers" valid:"required,url"`
	// Database is the database name for Jaeger service on the server
	Database string `mapstructure:"database_name"`
	// The port used when dialing to a cluster.
	Port int `mapstructure:"port"`
	// Authenticator contains the details of the authentication mechanism that is used for
	// connecting to a cluster.
	Authenticator Authenticator `mapstructure:"auth"`
}

// Authenticator holds the authentication properties needed to connect to a Clickhouse cluster.
type Authenticator struct {
	Basic BasicAuthenticator `mapstructure:"basic"`
}

// BasicAuthenticator holds the username and password for a password authenticator for a Clickhouse cluster.
type BasicAuthenticator struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

func DefaultConfiguration() *Configuration {
	return &Configuration{
		Connection: Connection{
			Servers:      []string{"127.0.0.1"},
			Port:         9000,
		},
	}
}
