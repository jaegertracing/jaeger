// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"fmt"
	"net/url"
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
// OTLP HTTP receiver. Used when the operator opts into the otlp_proxy block
// without overriding `target` themselves.
const DefaultOTLPProxyTarget = "http://127.0.0.1:4318"

// OTLPProxyConfig opts the query extension into mounting an HTTP reverse
// proxy at `/api/otlp/v1/*` that forwards to an OTel-collector OTLP HTTP
// receiver. The motivating use case is browser-side telemetry from the
// Jaeger UI: same-origin POSTs to the query port avoid the CORS preflight
// round-trip and the operator-side allow-list config that a cross-port
// alternative would require.
type OTLPProxyConfig struct {
	// Target is the base URL of the OTLP HTTP receiver to forward to.
	// The proxy strips the `/api/otlp` prefix before forwarding, so a POST
	// to `<query>/api/otlp/v1/traces` becomes `<target>/v1/traces`.
	Target string `mapstructure:"target" valid:"required"`
}

// Validate is a pure check; the default value is supplied by DefaultQueryOptions
// (see the OTLPProxyConfig type-level comment) so an operator who opts into the
// block without overrides still gets a valid Target.
func (c *OTLPProxyConfig) Validate() error {
	if c.Target == "" {
		return errors.New("otlp_proxy.target is required")
	}
	u, err := url.Parse(c.Target)
	if err != nil {
		return fmt.Errorf("otlp_proxy.target must be a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("otlp_proxy.target must use http or https scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("otlp_proxy.target must include a host")
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
	// OTLPProxy, when present, mounts a same-origin OTLP HTTP reverse proxy
	// at `/api/otlp/v1/*` on the query port. Absent (default) ⇒ no proxy route.
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
