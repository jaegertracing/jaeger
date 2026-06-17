// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/skills"
)

func (s *server) registerResources() {
	builtins := skills.BuiltinSkills(s.telset.Logger)
	if len(builtins) == 0 {
		s.telset.Logger.Warn("no built-in skills loaded; resources/list will return no skill resources")
		return
	}

	for _, sk := range builtins {
		s.mcpServer.AddResource(&mcp.Resource{
			URI:         "skill://" + sk.Name,
			Name:        sk.Name,
			Description: sk.Description,
			MIMEType:    "text/markdown",
		}, skillResourceHandler(sk.Body))
	}
	s.telset.Logger.Info("registered built-in skill resources", zap.Int("count", len(builtins)))
}

func skillResourceHandler(body string) mcp.ResourceHandler {
	return func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: "text/markdown", Text: body},
			},
		}, nil
	}
}
