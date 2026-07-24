// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"time"

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
	MaxSearchResults         int
	// MaxReadFileSize bounds the size (bytes) of a file served by read_skill.
	MaxReadFileSize int64
	// SkillsDir is an optional directory of operator-supplied skills on the
	// server disk, served by read_skill under custom/ alongside the built-in
	// skills. Empty means built-ins only.
	SkillsDir string
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
