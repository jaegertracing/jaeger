// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package testdata

// SimpleConfig is a test configuration struct
type SimpleConfig struct {
	// Port to listen on
	Port int `json:"port" mapstructure:"port"`

	// Host address
	Host string `json:"host" mapstructure:"host,required"`

	// Enable debug mode
	Debug bool `json:"debug,omitempty" mapstructure:"debug"`
}

// NestedConfig has nested structs
type NestedConfig struct {
	// Server configuration
	Server ServerConfig `json:"server" mapstructure:"server"`

	// Optional client config
	Client *ClientConfig `json:"client,omitempty" mapstructure:"client"`
}

// ServerConfig represents server settings
type ServerConfig struct {
	// Server port
	Port int `json:"port" mapstructure:"port" valid:"required"`
}

// ClientConfig represents client settings
type ClientConfig struct {
	// Timeout in seconds
	Timeout int `json:"timeout" mapstructure:"timeout"`
}
