// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"io/fs"

	"github.com/jaegertracing/jaeger/internal/version"
)

// Default tunables for the telemetry MCP server, preserved from the retired
// jaeger_mcp extension.
const (
	DefaultServerName               = "jaeger"
	DefaultMaxSpanDetailsPerRequest = 20
	DefaultMaxSearchResults         = 100
	DefaultMaxReadFileSize          = 512 * 1024

	// Upper bounds an operator may raise a tunable to, preserved from the
	// retired jaeger_mcp extension's config validation. They exist so a typo
	// cannot make one tool call allocate without limit. They sit next to the
	// defaults they pair with so the validation message and the documented
	// default cannot drift apart.
	MaxSpanDetailsPerRequestLimit = 100
	MaxSearchResultsLimit         = 1000
	MaxReadFileSizeLimit          = 10 * 1024 * 1024
)

// ServerParams holds what NewHandler needs beyond its dependencies: the tool
// limits and server identity, which are fixed by DefaultServerParams, and the
// operator-set parts of ai.mcp that the query server copies in (the custom
// skills directory and the transport options).
type ServerParams struct {
	ServerName               string
	ServerVersion            string
	MaxSpanDetailsPerRequest int
	MaxSearchResults         uint32
	// MaxReadFileSize bounds the size (bytes) of a file served by read_skill.
	MaxReadFileSize int64
	// CustomSkillsFS is the operator's skills directory (ai.skills_dir),
	// already opened by the caller, served by read_skill under custom/ beside
	// the built-in skills. Nil means none is configured, and only the built-ins
	// are served.
	CustomSkillsFS fs.FS
	// StreamableHTTP holds the options of the MCP Streamable HTTP transport the
	// endpoint is served over. They shape how requests are accepted and answered
	// on the wire and leave the tools untouched.
	StreamableHTTP StreamableHTTPConfig
}

// StreamableHTTPConfig configures the Streamable HTTP transport of the MCP
// endpoint: the go-sdk's StreamableHTTPOptions that an operator may need to
// change for a deployment. It is the `streamable_http` block of ai.mcp, so it
// carries the mapstructure tags the query extension's config loader uses.
type StreamableHTTPConfig struct {
	// DisableLocalhostProtection stops the endpoint from rejecting, with 403, a
	// request that arrives on a loopback connection with a non-loopback Host
	// header. That rejection guards an MCP server on a developer machine against
	// DNS rebinding; behind a reverse proxy or sidecar that dials 127.0.0.1 and
	// keeps the client's Host it would reject every request instead. Prefer making
	// the proxy rewrite Host or dial a non-loopback address; set this when the proxy
	// cannot be changed.
	DisableLocalhostProtection bool `mapstructure:"disable_localhost_protection" valid:"optional"`
	// JSONResponse makes the endpoint answer a POST that carries one JSON-RPC
	// message with a plain application/json body instead of a text/event-stream
	// body holding that one message. The stream form exists so a server can send
	// progress or its own requests before the result; Jaeger's tools send none, so
	// the two bodies carry the same result and a plain body is easier for proxies
	// and command-line clients to read. Clients must accept both forms, so this
	// changes nothing for them. A JSON-RPC batch is still answered with a stream.
	JSONResponse bool `mapstructure:"json_response" valid:"optional"`
}

// DefaultServerParams returns the limits the standalone jaeger_mcp extension used,
// so migrating operators see identical tool behaviour on the in-process endpoint.
func DefaultServerParams() ServerParams {
	ver := version.Get().GitVersion
	if ver == "" {
		ver = "dev"
	}
	return ServerParams{
		ServerName:               DefaultServerName,
		ServerVersion:            ver,
		MaxSpanDetailsPerRequest: DefaultMaxSpanDetailsPerRequest,
		MaxSearchResults:         DefaultMaxSearchResults,
		MaxReadFileSize:          DefaultMaxReadFileSize,
	}
}
