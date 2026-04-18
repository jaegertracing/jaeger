// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// ContextualToolsProvider gives read access to the AG-UI tools snapshot
// that the frontend attached to a chat request. It is keyed by ACP session
// ID so concurrent turns from different frontends each get their own
// snapshot without racing.
type ContextualToolsProvider interface {
	GetContextualToolsForSession(sessionID string) []any
}

// Extension is the interface that the jaegerquery extension implements.
// It allows other extensions to access the query service.
type Extension interface {
	extension.Extension
	// QueryService returns the v2 query service.
	QueryService() *querysvc.QueryService
	// TenancyManager returns the tenancy manager used by query endpoints.
	TenancyManager() *tenancy.Manager
	// ContextualToolsStore returns the store used to exchange AG-UI tools
	// between the AI chat gateway and the MCP extension.
	ContextualToolsStore() ContextualToolsProvider
}

// GetExtension retrieves the jaegerquery extension from the host.
func GetExtension(host component.Host) (Extension, error) {
	var id component.ID
	var comp component.Component
	for i, ext := range host.GetExtensions() {
		if i.Type() == componentType {
			id, comp = i, ext
			break
		}
	}
	if comp == nil {
		return nil, fmt.Errorf(
			"cannot find extension '%s' (make sure it's defined earlier in the config)",
			componentType,
		)
	}
	ext, ok := comp.(Extension)
	if !ok {
		return nil, fmt.Errorf("extension '%s' is not of expected type", id)
	}
	return ext, nil
}
