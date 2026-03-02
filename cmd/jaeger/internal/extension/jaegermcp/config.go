// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap/xconfmap"
)

// CORSConfig holds CORS configuration for the MCP server.
type CORSConfig struct {
	// AllowedOrigins is a list of origins that are allowed to make cross-origin requests.
	// Use ["*"] to allow any origin. Supports exact matches and wildcard patterns
	// (e.g., "http://*.example.com").
	AllowedOrigins []string `mapstructure:"allowed_origins"`

	// AllowedHeaders is a list of additional headers the client is allowed to use
	// in cross-origin requests. MCP protocol headers (Mcp-Session-Id,
	// Mcp-Protocol-Version, Last-Event-ID) are always permitted regardless of
	// this setting.
	AllowedHeaders []string `mapstructure:"allowed_headers"`
}

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

	// CORS configures cross-origin resource sharing for the MCP HTTP endpoint.
	// This is required for browser-based MCP clients like the MCP Inspector.
	CORS CORSConfig `mapstructure:"cors"`
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
