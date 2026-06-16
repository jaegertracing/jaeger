// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/skills"
)

// registerResources loads all built-in skills and registers each one as an MCP
// resource under the skill:// URI scheme. Agents discover available skills via
// resources/list and fetch full instructions via resources/read.
func (s *server) registerResources() {
	builtins := skills.BuiltinSkills()
	if len(builtins) == 0 {
		s.telset.Logger.Warn("no built-in skills loaded; resources/list will return no skill resources")
		return
	}

	for _, sk := range builtins {
		uri := "skill://" + sk.Name
		s.mcpServer.AddResource(&mcp.Resource{
			URI:         uri,
			Name:        sk.Name,
			Description: sk.Description,
			MIMEType:    "text/markdown",
		}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{
					{
						URI:      req.Params.URI,
						MIMEType: "text/markdown",
						Text:     sk.Body,
					},
				},
			}, nil
		})
	}
	s.telset.Logger.Info("registered built-in skill resources", zap.Int("count", len(builtins)))
}
