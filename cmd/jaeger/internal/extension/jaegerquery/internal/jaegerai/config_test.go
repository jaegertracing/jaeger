// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/config/configoptional"
)

// validConfig returns an Config that passes Validate; tests mutate the
// field they care about to exercise one rule at a time. The factory-default
// values mirror what configoptional.Default(...) seeds at runtime.
func validConfig() Config {
	return Config{
		AgentURL:            DefaultAgentURL,
		MaxRequestBodySize:  DefaultMaxRequestBodySize,
		HealthCheckInterval: DefaultHealthCheckInterval,
		HealthCheckTimeout:  DefaultHealthCheckTimeout,
	}
}

func TestConfigValidateAcceptsDefaults(t *testing.T) {
	cfg := validConfig()
	require.NoError(t, cfg.Validate())
}

func TestConfigValidateRejectsEmptyAgentURLWithoutMCP(t *testing.T) {
	cfg := validConfig()
	cfg.AgentURL = ""
	require.EqualError(t, cfg.Validate(), "ai requires agent_url (AI chat) or mcp (telemetry MCP tools)")
}

func TestConfigValidateAcceptsMCPOnly(t *testing.T) {
	cfg := validConfig()
	cfg.AgentURL = ""
	cfg.MCP = configoptional.Some(MCPConfig{})
	require.NoError(t, cfg.Validate())
}

func TestConfigValidateAcceptsSkillsDir(t *testing.T) {
	cfg := validConfig()
	cfg.MCP = configoptional.Some(MCPConfig{SkillsDir: "/etc/jaeger/skills"})
	require.NoError(t, cfg.Validate())
}

func TestConfigValidateRejectsAuthWithoutAuthenticator(t *testing.T) {
	cfg := validConfig()
	cfg.Auth = configoptional.Some(confighttp.AuthConfig{})
	require.EqualError(t, cfg.Validate(), "ai.auth.authenticator must name an extension")
}

func TestConfigValidateRejectsNonPositiveBodySize(t *testing.T) {
	for _, size := range []int64{0, -1} {
		cfg := validConfig()
		cfg.MaxRequestBodySize = size
		require.EqualError(t, cfg.Validate(), "ai.max_request_body_size must be a positive integer")
	}
}

func TestMCPConfigValidateAcceptsAbsentOrAbsoluteBaseURL(t *testing.T) {
	// Empty is valid — the announced URL is then resolved from AgentURL
	// (see ResolveMCPBaseURL); only an explicit override is validated here.
	require.NoError(t, (&MCPConfig{}).Validate())

	for _, u := range []string{
		"http://127.0.0.1:16686",
		"https://jaeger.example.com:16686",
		"https://jaeger.example.com",
	} {
		require.NoError(t, (&MCPConfig{BaseURL: u}).Validate(), "absolute URL %q must be accepted", u)
	}
}

func TestMCPConfigValidateRejectsRelativeBaseURL(t *testing.T) {
	// A scheme-less or relative value would be announced verbatim and fail at the
	// sidecar mid-turn — exactly what this field exists to prevent — so it must
	// fail at config load instead.
	const want = "ai.mcp.base_url must be an absolute URL including scheme and host, e.g. https://jaeger.example.com:16686"
	for _, u := range []string{
		"jaeger.example.com:16686", // no scheme
		"/api/ai/mcp",              // path only
		"http://",                  // no host
		"://nonsense",              // unparseable
	} {
		require.EqualError(t, (&MCPConfig{BaseURL: u}).Validate(), want, "relative/invalid URL %q must be rejected", u)
	}
}

func TestConfigResolveMCPBaseURL(t *testing.T) {
	const httpEndpoint = ":16686"
	tests := []struct {
		name       string
		mcpBaseURL string
		agentURL   string
		endpoint   string
		tls        bool
		want       string
	}{
		{
			name:       "explicit override wins over inference",
			mcpBaseURL: "https://jaeger.example.com:16686",
			agentURL:   "ws://sidecar.example.com:16688", // remote, but override wins
			endpoint:   httpEndpoint,
			want:       "https://jaeger.example.com:16686",
		},
		{
			name:     "co-located sidecar (localhost) infers localhost",
			agentURL: "ws://localhost:16688",
			endpoint: httpEndpoint,
			want:     "http://localhost:16686",
		},
		{
			// A server certificate carries a SAN for the name operators dial the
			// gateway by, essentially never for a loopback host, so an inferred
			// https:// URL would fail certificate verification at the sidecar.
			name:     "TLS infers nothing",
			agentURL: "ws://localhost:16688",
			endpoint: httpEndpoint,
			tls:      true,
			want:     "",
		},
		{
			name:       "explicit override still wins under TLS",
			mcpBaseURL: "https://jaeger.example.com:16686",
			agentURL:   "ws://localhost:16688",
			endpoint:   httpEndpoint,
			tls:        true,
			want:       "https://jaeger.example.com:16686",
		},
		{
			name:     "loopback IPv4 agent url infers localhost",
			agentURL: "ws://127.0.0.1:16688",
			endpoint: "0.0.0.0:16686",
			want:     "http://localhost:16686",
		},
		{
			name:     "wildcard IPv6 bind infers localhost",
			agentURL: "ws://localhost:16688",
			endpoint: "[::]:16686",
			want:     "http://localhost:16686",
		},
		{
			// Announced verbatim, not as "localhost": on a dual-stack host
			// "localhost" may resolve to the family the gateway did not bind.
			name:     "loopback IPv4 bind is announced verbatim",
			agentURL: "ws://localhost:16688",
			endpoint: "127.0.0.1:16686",
			want:     "http://127.0.0.1:16686",
		},
		{
			// JoinHostPort has to bracket the host, or the URL is unparseable.
			name:     "loopback IPv6 bind is announced bracketed",
			agentURL: "ws://localhost:16688",
			endpoint: "[::1]:16686",
			want:     "http://[::1]:16686",
		},
		{
			name:     "localhost bind infers localhost",
			agentURL: "ws://localhost:16688",
			endpoint: "localhost:16686",
			want:     "http://localhost:16686",
		},
		{
			// Bound to one specific interface, nothing answers on loopback, so the
			// co-located sidecar still cannot reach us there.
			name:     "specific non-loopback interface bind infers nothing",
			agentURL: "ws://localhost:16688",
			endpoint: "10.0.0.5:16686",
			want:     "",
		},
		{
			// Resolving the host, rather than string-matching "localhost", is what
			// makes these agree with what a dial would do.
			name:     "agent url host is matched case-insensitively",
			agentURL: "ws://LOCALHOST:16688",
			endpoint: httpEndpoint,
			want:     "http://localhost:16686",
		},
		{
			// ".invalid" is reserved as non-resolvable (RFC 2606), so the bound
			// host is not something we can reason about.
			name:     "unresolvable bind host infers nothing",
			agentURL: "ws://localhost:16688",
			endpoint: "nowhere.invalid:16686",
			want:     "",
		},
		{
			name:     "portless agent url is co-located",
			agentURL: "ws://localhost",
			endpoint: httpEndpoint,
			want:     "http://localhost:16686",
		},
		{
			name:     "loopback IPv6 agent url infers localhost",
			agentURL: "ws://[::1]:16688",
			endpoint: httpEndpoint,
			want:     "http://localhost:16686",
		},
		{
			name:     "remote sidecar infers nothing",
			agentURL: "ws://sidecar.example.com:16688",
			endpoint: httpEndpoint,
			want:     "",
		},
		{
			name:     "empty agent url infers nothing",
			agentURL: "",
			endpoint: httpEndpoint,
			want:     "",
		},
		{
			name:     "unparseable agent url infers nothing",
			agentURL: "://nonsense",
			endpoint: httpEndpoint,
			want:     "",
		},
		{
			name:     "endpoint without a port infers nothing",
			agentURL: "ws://localhost:16688",
			endpoint: "localhost", // no host:port split
			want:     "",
		},
		{
			name:     "dynamic port endpoint infers nothing",
			agentURL: "ws://localhost:16688",
			endpoint: ":0",
			want:     "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				AgentURL: tc.agentURL,
				MCP:      configoptional.Some(MCPConfig{BaseURL: tc.mcpBaseURL}),
			}
			assert.Equal(t, tc.want, cfg.ResolveMCPBaseURL(context.Background(), tc.endpoint, tc.tls))
		})
	}
}

func TestConfigValidateAcceptsZeroHealthCheckIntervalAsDisable(t *testing.T) {
	cfg := validConfig()
	cfg.HealthCheckInterval = 0
	// Timeout becomes irrelevant when the checker is disabled; the validator
	// must not require a positive timeout in that case.
	cfg.HealthCheckTimeout = 0
	require.NoError(t, cfg.Validate())
}

func TestConfigValidateRejectsNegativeHealthCheckInterval(t *testing.T) {
	cfg := validConfig()
	cfg.HealthCheckInterval = -time.Second
	require.EqualError(t, cfg.Validate(),
		"ai.health_check_interval must not be negative (0 disables the health checker)")
}

func TestConfigValidateRejectsNonPositiveHealthCheckTimeoutWhenEnabled(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		cfg := validConfig()
		cfg.HealthCheckTimeout = timeout
		require.EqualError(t, cfg.Validate(),
			"ai.health_check_timeout must be positive when health_check_interval is positive")
	}
}

// TestHostIPUsesFirstResolvedAddress covers the name-resolution path. "localhost" is
// short-circuited and IP literals never reach the resolver, so a stubbed lookup is
// the only way to reach it without depending on the environment's DNS.
func TestHostIPUsesFirstResolvedAddress(t *testing.T) {
	restore := lookupIPAddr
	t.Cleanup(func() { lookupIPAddr = restore })
	lookupIPAddr = func(_ context.Context, host string) ([]net.IPAddr, error) {
		assert.Equal(t, "sidecar.internal", host, "the host is passed through verbatim")
		return []net.IPAddr{{IP: net.IPv4(127, 0, 0, 5)}, {IP: net.IPv4(10, 0, 0, 1)}}, nil
	}
	assert.Equal(t, "127.0.0.5", hostIP(context.Background(), "sidecar.internal").String(),
		"the first resolved address decides")
}

// TestHostIPSkipsResolverForLocalhost pins the fast path: ai.agent_url defaults to
// ws://localhost:16688, so resolving it would put a lookup on every default-config
// startup. A stub that fails the test if called proves it is never consulted.
func TestHostIPSkipsResolverForLocalhost(t *testing.T) {
	restore := lookupIPAddr
	t.Cleanup(func() { lookupIPAddr = restore })
	lookupIPAddr = func(_ context.Context, host string) ([]net.IPAddr, error) {
		assert.Fail(t, "resolver must not be consulted", "called for %q", host)
		return nil, nil
	}
	for _, host := range []string{"localhost", "LOCALHOST", "127.0.0.1", "::1"} {
		ip := hostIP(context.Background(), host)
		require.NotNil(t, ip, "%q must resolve without the resolver", host)
		assert.True(t, ip.IsLoopback(), "%q is loopback", host)
	}
}

func TestConfigRejectsDuplicateAgentHeaders(t *testing.T) {
	cfg := Config{
		AgentURL:           "ws://localhost:16688",
		MaxRequestBodySize: 1 << 20,
		AgentHeaders: configopaque.MapList{
			{Name: "X-Secret-Key", Value: "one"},
			{Name: "X-Secret-Key", Value: "two"},
		},
	}
	require.ErrorContains(t, cfg.Validate(), "ai.agent_headers")
}

func TestConfigAcceptsAgentHeaders(t *testing.T) {
	cfg := Config{
		AgentURL:           "ws://localhost:16688",
		MaxRequestBodySize: 1 << 20,
		AgentHeaders: configopaque.MapList{
			{Name: "X-Secret-Key", Value: "one"},
		},
	}
	require.NoError(t, cfg.Validate())
}
