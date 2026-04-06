// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"time"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configoptional"

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

type AIConfig struct {
	// AgentURL is the WebSocket endpoint of an agent that supports ACP.
	// See https://agentclientprotocol.com/
	AgentURL string `mapstructure:"agent_url" valid:"required"`
	// WaitForTurnTimeout is the max wait time for receiving turn completion after prompt succeeds.
	WaitForTurnTimeout time.Duration `mapstructure:"wait_for_turn_timeout" valid:"optional"`
}

// QueryOptions holds configuration for query service shared with jaeger-v2
type QueryOptions struct {
	// BasePath is the base path for all HTTP routes.
	BasePath string `mapstructure:"base_path"`
	// UIConfig contains configuration related to the Jaeger UIConfig.
	UIConfig UIConfig `mapstructure:"ui"`
	// BearerTokenPropagation activate/deactivate bearer token propagation to storage.
	BearerTokenPropagation bool `mapstructure:"bearer_token_propagation"`
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
			AgentURL:           "ws://localhost:9000",
			WaitForTurnTimeout: 180 * time.Second,
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
