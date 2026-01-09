// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// Extension is the interface that the jaegerquery extension implements.
// It allows other extensions to access the storage readers.
type Extension interface {
	extension.Extension
	// V1SpanReader returns the v1 span reader.
	V1SpanReader() spanstore.Reader
	// V2TraceReader returns the v2 trace reader.
	V2TraceReader() tracestore.Reader
	// DependencyReader returns the dependency reader.
	DependencyReader() depstore.Reader
}

// GetExtension retrieves the jaegerquery extension from the host.
func GetExtension(host component.Host) (Extension, error) {
	return findExtension(host)
}

// findExtension locates the jaegerquery extension in the host.
func findExtension(host component.Host) (Extension, error) {
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
