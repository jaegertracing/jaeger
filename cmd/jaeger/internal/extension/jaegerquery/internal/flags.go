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

// Defaults for AIConfig fields. Applied when the field is left at its zero
// value (or, for AgentURL, when DefaultQueryOptions seeds the configoptional
// default).
const (
	DefaultAIAgentURL                  = "ws://localhost:16688"
	DefaultAIMaxRequestBodySize  int64 = 1 << 20 // 1 MiB
	DefaultAIHealthCheckInterval       = 30 * time.Second
	DefaultAIHealthCheckTimeout        = 2 * time.Second
)

// AIConfig is the AI-related slice of QueryOptions. All defaults are seeded
// by DefaultQueryOptions via configoptional.Default, and a user's partial
// YAML block overlays only the fields they specify (configoptional unmarshals
// onto the seeded value), so unset fields keep their default. Validate is
// therefore a pure check — it does not mutate the receiver.
type AIConfig struct {
	// AgentURL is the WebSocket endpoint of an ACP-compatible agent sidecar.
	// For example, ws://localhost:16688
	// See https://agentclientprotocol.com/
	AgentURL string `mapstructure:"agent_url" valid:"required"`
	// MaxRequestBodySize limits the chat-handler request body. Must be positive.
	MaxRequestBodySize int64 `mapstructure:"max_request_body_size" valid:"optional"`
	// HealthCheckInterval controls how often the AI health checker contacts
	// the sidecar to determine if the chat surface should be advertised to
	// the UI. Set to 0 to disable the health checker (advertised capability
	// stays at false); negative values are rejected.
	HealthCheckInterval time.Duration `mapstructure:"health_check_interval" valid:"optional"`
	// HealthCheckTimeout is the per-check timeout. Must be positive when
	// HealthCheckInterval > 0; ignored when the checker is disabled.
	HealthCheckTimeout time.Duration `mapstructure:"health_check_timeout" valid:"optional"`
}

// DefaultOTLPProxyTarget is the loopback endpoint of the bundled OTel-collector
// OTLP HTTP receiver.
const DefaultOTLPProxyTarget = "http://127.0.0.1:4318"

// OTLPProxyConfig mounts an HTTP reverse proxy at `<basePath>/api/otlp/v1/*`
// that strips the `/api/otlp` prefix and forwards to Target. Intended for
// same-origin browser telemetry from the SPA — POSTs to the query port
// avoid the CORS preflight a cross-port OTLP receiver would need.
type OTLPProxyConfig struct {
	// Target is the base URL of the OTLP HTTP receiver to forward to.
	Target string `mapstructure:"target" valid:"required"`
}

func (c *OTLPProxyConfig) Validate() error {
	if c.Target == "" {
		return errors.New("otlp_proxy.target is required")
	}
	return nil
}

// Validate is a pure check; defaults are supplied by DefaultQueryOptions
// (see the AIConfig type-level comment) so by the time Validate runs the
// caller's struct already has sensible values for any field they omitted.
func (c *AIConfig) Validate() error {
	if c.AgentURL == "" {
		return errors.New("ai.agent_url is required")
	}
	if c.MaxRequestBodySize <= 0 {
		return errors.New("ai.max_request_body_size must be a positive integer")
	}
	if c.HealthCheckInterval < 0 {
		return errors.New("ai.health_check_interval must not be negative (0 disables the health checker)")
	}
	if c.HealthCheckInterval > 0 && c.HealthCheckTimeout <= 0 {
		return errors.New("ai.health_check_timeout must be positive when health_check_interval is positive")
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
	// OTLPProxy, when present, mounts an OTLP HTTP reverse proxy — see OTLPProxyConfig.
	OTLPProxy configoptional.Optional[OTLPProxyConfig] `mapstructure:"otlp_proxy"`
}

func DefaultQueryOptions() QueryOptions {
	return QueryOptions{
		MaxClockSkewAdjust: 0, // disabled by default
		AI: configoptional.Default(AIConfig{
			AgentURL:            DefaultAIAgentURL,
			MaxRequestBodySize:  DefaultAIMaxRequestBodySize,
			HealthCheckInterval: DefaultAIHealthCheckInterval,
			HealthCheckTimeout:  DefaultAIHealthCheckTimeout,
		}),
		OTLPProxy: configoptional.Default(OTLPProxyConfig{
			Target: DefaultOTLPProxyTarget,
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
