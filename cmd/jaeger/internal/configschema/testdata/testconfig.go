// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package testdata

// TestConfig is a simple struct for testing schema generation
type TestConfig struct {
	// Name is the name of the configuration
	Name string `mapstructure:"name"`
	
	// Port is the port to listen on
	Port int `mapstructure:"port"`
	
	// Enabled determines if the service is enabled
	Enabled bool `mapstructure:"enabled"`
	
	// Tags are key-value pairs for metadata
	Tags map[string]string `mapstructure:"tags"`
	
	// Addresses is a list of addresses to connect to
	Addresses []string `mapstructure:"addresses"`
	
	// NestedConfig is a nested configuration
	NestedConfig NestedConfig `mapstructure:"nested_config"`
	
	// OptionalConfig is an optional nested configuration
	OptionalConfig *NestedConfig `mapstructure:"optional_config,omitempty"`
	
	// Deprecated: This field is deprecated
	DeprecatedField string `mapstructure:"deprecated_field"`
}

// NestedConfig is a nested configuration struct for testing
type NestedConfig struct {
	// Timeout is the timeout in seconds
	Timeout int `mapstructure:"timeout"`
	
	// RetryCount is the number of retries
	RetryCount int `mapstructure:"retry_count"`
}
