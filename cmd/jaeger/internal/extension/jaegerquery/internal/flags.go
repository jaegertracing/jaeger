// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"time"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configoptional"

	"github.com/jaegertracing/jaeger/internal/headerforwarding"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/ports"
)

type UIConfig struct {
	// ConfigFile is the path to a configuration file for the UI.
	ConfigFile string `mapstructure:"config_file" valid:"optional"`
	// AssetsPath is the path for the static assets for the UI (https://github.com/uber/jaeger-ui).
	AssetsPath string `mapstructure:"assets_path" valid:"optional" `
	// LogAccess tells static handler to log access to static assets, useful in debugging.
	LogAccess bool `mapstructure:"log_access" valid:"optional"`
}

// DefaultMaxRequestBodySize is the fallback limit applied when
// AIConfig.MaxRequestBodySize is left unset (zero).
const DefaultMaxRequestBodySize int64 = 1 << 20 // 1 MiB

// DefaultHealthCheckInterval and DefaultHealthCheckTimeout are the fallback
// values applied when AIConfig.HealthCheckInterval / HealthCheckTimeout are
// left unset (zero).
const (
	DefaultHealthCheckInterval = 5 * time.Second
	DefaultHealthCheckTimeout  = 2 * time.Second
)

type AIConfig struct {
	// AgentURL is the WebSocket endpoint of an ACP-compatible agent sidecar.
	// For example, ws://localhost:16688
	// See https://agentclientprotocol.com/
	AgentURL string `mapstructure:"agent_url" valid:"required"`
	// A value of 0 selects DefaultMaxRequestBodySize; negative values are rejected.
	MaxRequestBodySize int64 `mapstructure:"max_request_body_size" valid:"optional"`
	// HealthCheckInterval controls how often the AI health checker contacts
	// the sidecar to determine if the chat surface should be advertised to
	// the UI. A value of 0 selects DefaultHealthCheckInterval; a negative
	// value disables checking entirely and pins the advertised capability
	// to false.
	HealthCheckInterval time.Duration `mapstructure:"health_check_interval" valid:"optional"`
	// HealthCheckTimeout is the per-check timeout. A value of 0 selects
	// DefaultHealthCheckTimeout; negative values are rejected.
	HealthCheckTimeout time.Duration `mapstructure:"health_check_timeout" valid:"optional"`
}

// Validate checks the AI config and applies defaults in place when fields
// are left at their zero value; the pointer receiver is required so the
// defaults persist back to the caller's config.
func (c *AIConfig) Validate() error {
	if c.MaxRequestBodySize < 0 {
		return errors.New("ai.max_request_body_size must be a non-negative integer")
	}
	if c.MaxRequestBodySize == 0 {
		c.MaxRequestBodySize = DefaultMaxRequestBodySize
	}
	if c.HealthCheckTimeout < 0 {
		return errors.New("ai.health_check_timeout must be a non-negative duration")
	}
	if c.HealthCheckTimeout == 0 {
		c.HealthCheckTimeout = DefaultHealthCheckTimeout
	}
	if c.HealthCheckInterval == 0 {
		c.HealthCheckInterval = DefaultHealthCheckInterval
	}
	return nil
}

// QueryOptions holds configuration for query service shared with jaeger-v2
type QueryOptions struct {
	// BasePath is the base path for all HTTP routes.
	BasePath string `mapstructure:"base_path"`
	// UIConfig contains configuration related to the Jaeger UIConfig.
	UIConfig UIConfig `mapstructure:"ui"`
	// BearerTokenPropagation activate/deactivate bearer token propagation to storage.
	BearerTokenPropagation bool `mapstructure:"bearer_token_propagation"`
	// HeaderForwarding lists additional request headers to extract and forward to the storage backend.
	HeaderForwarding []headerforwarding.ForwardedHeader `mapstructure:"header_forwarding"`
	// Tenancy holds the multi-tenancy configuration.
	Tenancy tenancy.Options `mapstructure:"multi_tenancy"`
	// MaxClockSkewAdjust is the maximum duration by which jaeger-query will adjust a span.
	MaxClockSkewAdjust time.Duration `mapstructure:"max_clock_skew_adjust"  valid:"optional"`
	// MaxTraceSize is the maximum number of spans allowed per trace. A value of 0 (default) means unlimited.
	// If a trace has more spans than this limit, it will be truncated and a warning will be added.
	MaxTraceSize int `mapstructure:"max_trace_size" valid:"optional"`
	// EnableTracing determines whether traces will be emitted by jaeger-query.
	EnableTracing bool `mapstructure:"enable_tracing"`
	// HTTP holds the HTTP configuration that the query service uses to serve requests.
	HTTP confighttp.ServerConfig `mapstructure:"http"`
	// GRPC holds the GRPC configuration that the query service uses to serve requests.
	GRPC configgrpc.ServerConfig `mapstructure:"grpc"`
	// AI holds configuration related to Jaeger AI gateway integration.
	AI configoptional.Optional[AIConfig] `mapstructure:"ai"`
}

func DefaultQueryOptions() QueryOptions {
	return QueryOptions{
		MaxClockSkewAdjust: 0, // disabled by default
		AI: configoptional.Default(AIConfig{
			AgentURL:            "ws://localhost:16688",
			MaxRequestBodySize:  DefaultMaxRequestBodySize,
			HealthCheckInterval: DefaultHealthCheckInterval,
			HealthCheckTimeout:  DefaultHealthCheckTimeout,
		}),
		HTTP: confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ports.PortToHostPort(ports.QueryHTTP),
				Transport: confignet.TransportTypeTCP,
			},
		},
		GRPC: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ports.PortToHostPort(ports.QueryGRPC),
				Transport: confignet.TransportTypeTCP,
			},
		},
	}
}
