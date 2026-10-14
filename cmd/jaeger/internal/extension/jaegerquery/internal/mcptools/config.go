// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"io/fs"
	"os"
	"strings"
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

	// captureContentEnvVar is the switch OpenTelemetry GenAI instrumentations
	// use for recording message and tool-call content, which they leave off
	// unless it is set to "true".
	captureContentEnvVar = "OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT"
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
	// CaptureContent records tool-call arguments and results
	// (gen_ai.tool.call.arguments and gen_ai.tool.call.result) on the MCP
	// tool-call spans. It is opt-in because a result is trace data read through
	// the query service, while those spans go wherever Jaeger exports its own
	// telemetry, whose readers may not be allowed to see that data.
	CaptureContent bool
}

// DefaultConfig returns the Config the standalone jaeger_mcp extension used, so
// migrating operators see identical tool behaviour on the in-process endpoint.
// CaptureContent is taken from OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT.
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
		CaptureContent:           strings.EqualFold(os.Getenv(captureContentEnvVar), "true"),
	}
}
