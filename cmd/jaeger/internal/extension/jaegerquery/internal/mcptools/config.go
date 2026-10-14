// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"io/fs"
	"time"

	"github.com/jaegertracing/jaeger/internal/headerforwarding"
	"github.com/jaegertracing/jaeger/internal/version"
)

// Default tunables for the telemetry MCP server, preserved from the retired
// jaeger_mcp extension.
const (
	DefaultServerName               = "jaeger"
	DefaultMaxSpanDetailsPerRequest = 20
	DefaultMaxSearchResults         = 100
	DefaultMaxReadFileSize          = 512 * 1024

	// mcpSessionTimeout caps an idle MCP session. The streamable handler keeps
	// per-MCP-session state for SSE resumption and stream-id correlation.
	mcpSessionTimeout = 5 * time.Minute
)

// Config holds the tunables for the in-process telemetry MCP server. It is the
// non-transport slice of the retired jaeger_mcp extension's config — the HTTP
// listener is gone because the handler now mounts on jaeger-query's own mux.
type Config struct {
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
	// HeaderForwarding is jaeger-query's header_forwarding. A request on an MCP
	// session must carry the same values of each of these headers as the
	// request that opened it, including carrying none.
	HeaderForwarding []headerforwarding.ForwardedHeader
	// BearerTokenPropagation is jaeger-query's bearer_token_propagation. When
	// set, each request on an MCP session reaches storage with its own token.
	BearerTokenPropagation bool
	// Authenticated is set when an HTTP server authenticator sits in front of
	// the handler (jaeger-query's http.auth). The session is then bound to
	// that authenticator's principal when it sets client.Info.Auth, and
	// otherwise to the headers and auth.request_params it was given. An
	// authenticator that stores the caller only in a context value would
	// otherwise leave the opener's context reusable by any later holder of
	// the session ID.
	Authenticated bool
	// AuthRequestParameters are jaeger-query's http.auth.request_params: the
	// query keys confighttp passes to the authenticator along with the headers.
	AuthRequestParameters []string
}

// DefaultConfig returns the Config the standalone jaeger_mcp extension used, so
// migrating operators see identical tool behaviour on the in-process endpoint.
func DefaultConfig() Config {
	ver := version.Get().GitVersion
	if ver == "" {
		ver = "dev"
	}
	return Config{
		ServerName:               DefaultServerName,
		ServerVersion:            ver,
		MaxSpanDetailsPerRequest: DefaultMaxSpanDetailsPerRequest,
		MaxSearchResults:         DefaultMaxSearchResults,
		MaxReadFileSize:          DefaultMaxReadFileSize,
	}
}
