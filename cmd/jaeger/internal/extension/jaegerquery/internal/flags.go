// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configopaque"
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
	// Optional: leave empty (and set the mcp block) to expose the telemetry MCP
	// endpoint without the AI chat surface.
	AgentURL string `mapstructure:"agent_url" valid:"optional"`
	// AgentHeaders are extra HTTP headers sent on the agent WebSocket
	// handshake, for agents that require authentication. The header name is the
	// agent's choice, not ours (Goose expects "X-Secret-Key", others expect
	// "Authorization"), so this is a map rather than a token field: a new scheme
	// needs configuration instead of code.
	//
	// The values are configopaque.String, the collector's own secret-safe type,
	// so a credential stays out of logs and config dumps by construction rather
	// than by everyone remembering not to print it. Prefer env expansion
	// (${env:VAR}) over inline values. Ignored unless AgentURL is set.
	AgentHeaders configopaque.MapList `mapstructure:"agent_headers" valid:"optional"`
	// MCP exposes Jaeger telemetry MCP server at <basePath>/api/ai/mcp/ on
	// the query port. Present enables it, absent disables it — an empty block
	// (`mcp: {}`) is enough. It replaces the retired standalone jaeger_mcp
	// extension (which served :16687); point Cursor/IDE MCP clients at the query
	// port instead. Independent of AgentURL.
	MCP configoptional.Optional[MCPConfig] `mapstructure:"mcp"`
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

// MCPConfig configures the telemetry MCP endpoint. Its presence on AIConfig is
// what enables the endpoint, so both fields are optional and an empty block is
// the common case.
type MCPConfig struct {
	// BaseURL is the externally-reachable scheme+authority a sidecar uses to
	// dial the turn-scoped MCP endpoint, e.g. "https://jaeger.example.com:16686".
	// The gateway announces "<BaseURL><basePath>/api/ai/mcp/<mcpRouteID>/" to
	// the sidecar in the session/new request.
	//
	// Optional override. If left empty, the gateway infers its own loopback address
	// when the sidecar is co-located (AgentURL is a loopback address), the query
	// server is bound to loopback or a wildcard, and TLS is off — the common
	// single-host deployment, which then needs no configuration at all (see
	// resolveMCPBaseURL). Set this whenever the sidecar reaches the gateway at
	// some other address — behind a proxy, in another network namespace, with TLS
	// terminated elsewhere, or forwarded into a container — none of which the
	// query server can infer. Ignored unless AgentURL is also set.
	BaseURL string `mapstructure:"base_url" valid:"optional"`
	// SkillsDir is a directory of operator-supplied skill playbooks on the query
	// server's disk, served by the read_skill MCP tool under custom/ beside the
	// built-in skills, so an installation can add its own without rebuilding
	// Jaeger. See mcptools/README.md for the layout it expects. Empty (the
	// default) serves the built-in skills only.
	SkillsDir string `mapstructure:"skills_dir" valid:"optional"`
	// MaxSpanDetailsPerRequest caps how many spans a single get_span_details,
	// get_trace_errors, or get_trace_topology call returns. It is the main lever
	// on how much raw JSON a tool pushes into the model's context window, so
	// operators running smaller-context models need to lower it. Zero (the
	// default) keeps mcptools.DefaultMaxSpanDetailsPerRequest.
	MaxSpanDetailsPerRequest int `mapstructure:"max_span_details_per_request" valid:"optional"`
	// MaxSearchResults caps search_traces' search_depth. A caller asking for more
	// is clamped to this value rather than rejected. Zero (the default) keeps
	// mcptools.DefaultMaxSearchResults.
	MaxSearchResults int `mapstructure:"max_search_results" valid:"optional"`
	// MaxReadFileSize bounds the size in bytes of a skill file served by
	// read_skill. Zero (the default) keeps mcptools.DefaultMaxReadFileSize.
	MaxReadFileSize int64 `mapstructure:"max_read_file_size" valid:"optional"`
}

func (c *MCPConfig) Validate() error {
	// Negative limits would silently disable the tools they bound rather than
	// restrict them, so reject them at config load. Zero stays legal and means
	// "keep the built-in default".
	if c.MaxSpanDetailsPerRequest < 0 {
		return errors.New("ai.mcp.max_span_details_per_request must not be negative")
	}
	if c.MaxSearchResults < 0 {
		return errors.New("ai.mcp.max_search_results must not be negative")
	}
	if c.MaxReadFileSize < 0 {
		return errors.New("ai.mcp.max_read_file_size must not be negative")
	}

	if c.BaseURL == "" {
		return nil
	}
	// Reject anything we cannot turn into a dialable absolute URL. A relative
	// or scheme-less value would be announced verbatim and fail at the
	// sidecar, which is exactly the mid-turn failure this field exists to
	// avoid — so fail fast at config load instead.
	u, err := url.Parse(c.BaseURL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return errors.New("ai.mcp.base_url must be an absolute URL including scheme and host, e.g. https://jaeger.example.com:16686")
	}
	return nil
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
	if c.AgentURL == "" && !c.MCP.HasValue() {
		return errors.New("ai requires agent_url (AI chat) or mcp (telemetry MCP tools)")
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
	// MapList permits repeated names on the wire; a duplicated header would
	// silently keep only one value, so reject it here rather than guess which.
	if err := c.AgentHeaders.Validate(); err != nil {
		return fmt.Errorf("ai.agent_headers: %w", err)
	}
	// MCPConfig.Validate is reached by the collector's config walk on its own,
	// the same way OTLPProxyConfig's is; delegating to it here would only
	// duplicate the check and drop the "mcp:" path segment from the message.
	return nil
}

// resolveMCPBaseURL returns the base URL the gateway announces for the turn-scoped
// MCP endpoint, or "" to announce no HTTP transport. An explicit base_url always
// wins. Otherwise the gateway infers its own loopback address, which requires every
// leg of the round trip to hold:
//
//   - The sidecar must be co-located: AgentURL is a loopback address, so the
//     sidecar shares this network namespace and its "localhost" is ours.
//   - The query server must actually be listening on loopback: a wildcard bind
//     (":16686", "0.0.0.0", "::") or a loopback bind. Bound to one specific
//     interface, say "10.0.0.5:16686", nothing answers on loopback.
//   - TLS must be off. A server certificate carries a SAN for the name operators
//     dial the gateway by, essentially never for an inferred loopback host, so an
//     inferred https:// URL fails certificate verification at the sidecar.
//
// If any leg is unmet, nothing is announced until an operator sets base_url. The
// gateway declines rather than guessing: a specific-interface bind or TLS is positive
// evidence that an inferred loopback URL is wrong, and there is nothing else here to
// derive a correct one from.
//
// One case this cannot detect: loopback forwarded across namespaces (a sidecar in a
// container published with "-p 127.0.0.1:16688:16688", or "kubectl port-forward").
// The gateway reaches the sidecar over loopback, but the sidecar's loopback is its
// own namespace, where the gateway is not listening. That is indistinguishable from
// genuine co-location here, and needs an explicit base_url.
//
// httpEndpoint is the query server's own HTTP host:port and tlsEnabled its own TLS
// setting; neither is derived from AgentURL, which only gates the inference.
func (c *AIConfig) resolveMCPBaseURL(ctx context.Context, httpEndpoint string, tlsEnabled bool) string {
	mcp := c.MCP.Get()
	if mcp == nil {
		return "" // MCP is off, so there is no endpoint to announce
	}
	if mcp.BaseURL != "" {
		return mcp.BaseURL
	}
	if tlsEnabled {
		return ""
	}
	if !isLoopbackURL(ctx, c.AgentURL) {
		return ""
	}
	boundHost, port, err := net.SplitHostPort(httpEndpoint)
	if err != nil || port == "" || port == "0" {
		// No fixed port to advertise (unset, or a dynamic ":0" resolved only at
		// listen time), so we cannot build a dialable URL — announce nothing.
		return ""
	}
	host := loopbackAnnounceHost(ctx, boundHost)
	if host == "" {
		return ""
	}
	// JoinHostPort, not concatenation: an IPv6 host has to be bracketed, or
	// "http://::1:16686" is not a parseable URL.
	return "http://" + net.JoinHostPort(host, port)
}

// loopbackAnnounceHost returns the host to announce for the query server's bound
// host, or "" when the gateway is not reachable over loopback. A wildcard bind
// listens on every interface including loopback, so "localhost" is announced. A
// loopback bind is announced verbatim rather than as "localhost", because on a
// dual-stack host "localhost" may resolve to the family the gateway did not bind
// (127.0.0.1 against a "[::1]" bind, or the reverse).
func loopbackAnnounceHost(ctx context.Context, boundHost string) string {
	if boundHost == "" {
		return "localhost" // ":16686" — wildcard, every interface
	}
	ip := hostIP(ctx, boundHost)
	switch {
	case ip == nil:
		return "" // not resolvable, so not something we can reason about
	case ip.IsUnspecified():
		return "localhost" // "0.0.0.0" / "::" — wildcard
	case ip.IsLoopback():
		return boundHost
	default:
		return "" // one specific non-loopback interface
	}
}

// isLoopbackURL reports whether rawURL's host is a loopback address. Used to detect
// a co-located sidecar from AgentURL.
func isLoopbackURL(ctx context.Context, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	ip := hostIP(ctx, u.Hostname())
	return ip != nil && ip.IsLoopback()
}

// hostLookupTimeout bounds name resolution in hostIP on top of whatever deadline
// the caller's context carries. The resolver is consulted while the query server is
// starting up, so an unreachable or slow DNS server must not stall startup; giving
// up means "not loopback", which only declines to infer.
const hostLookupTimeout = 2 * time.Second

// lookupIPAddr is net.DefaultResolver's lookup, indirected so tests can exercise the
// name-resolution path without depending on what DNS or /etc/hosts happens to answer
// in the environment the tests run in.
var lookupIPAddr = net.DefaultResolver.LookupIPAddr

// hostIP resolves a host — a name or an IP literal — to an IP address, or nil if it
// does not resolve. Resolving, rather than string-matching "localhost", is what
// makes the loopback checks agree with what a dial would actually do: it accepts
// any spelling of a name that maps to loopback, "LOCALHOST" or an /etc/hosts alias
// included, instead of the one spelling a comparison would hard-code.
func hostIP(ctx context.Context, host string) net.IP {
	if host == "" {
		return nil
	}
	// An IP literal needs no resolver at all, which keeps the common case off the
	// network entirely. netip.ParseAddr rather than net.ParseIP so that an IPv6
	// zone, "::1%lo0", parses instead of being rejected.
	if addr, err := netip.ParseAddr(host); err == nil {
		return net.IP(addr.AsSlice())
	}
	// "localhost" is the one name worth short-circuiting: it is the host in
	// DefaultAIAgentURL, so resolving it would put a lookup on every default-config
	// startup to learn what every resolver on every platform already agrees on. Any
	// other name still goes to the resolver below.
	if strings.EqualFold(host, "localhost") {
		// A fresh value, not net.IPv6loopback: that is a mutable package var, and
		// callers have no reason to be handed an alias of it.
		return net.IPv4(127, 0, 0, 1)
	}
	ctx, cancel := context.WithTimeout(ctx, hostLookupTimeout)
	defer cancel()
	addrs, err := lookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return nil
	}
	return addrs[0].IP
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
