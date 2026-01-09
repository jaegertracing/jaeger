// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap/xconfmap"
)

// Config represents the configuration for the Jaeger MCP server extension.
type Config struct {
	// HTTP contains the HTTP server configuration for the MCP protocol endpoint.
	HTTP confighttp.ServerConfig `mapstructure:"http"`

	// ServerName is the name of the MCP server for protocol identification.
	ServerName string `mapstructure:"server_name"`

	// ServerVersion is the version of the MCP server.
	ServerVersion string `mapstructure:"server_version" valid:"required"`

	// MaxSpanDetailsPerRequest limits the number of spans that can be fetched in a single request.
	MaxSpanDetailsPerRequest int `mapstructure:"max_span_details_per_request" valid:"range(1|100)"`

	// MaxSearchResults limits the number of trace search results.
	MaxSearchResults int `mapstructure:"max_search_results" valid:"range(1|1000)"`
}

// Validate checks if the configuration is valid.
func (cfg *Config) Validate() error {
	// if cfg.ServerVersion == "" {
	// 	cfg.ServerVersion = version.Get().GitVersion
	// }
	_, err := govalidator.ValidateStruct(cfg)
	return err
}

var _ xconfmap.Validator = (*Config)(nil)
