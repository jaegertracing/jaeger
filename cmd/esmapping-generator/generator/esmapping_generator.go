// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app/renderer"
	"github.com/jaegertracing/jaeger/internal/storage/v1/es/mappings"
	"github.com/jaegertracing/jaeger/pkg/es"
)

func GenerateMappings(options app.Options) (string, error) {
	if _, err := mappings.MappingTypeFromString(options.Mapping); err != nil {
		return "", fmt.Errorf("invalid mapping type '%s': please pass either 'jaeger-service' or 'jaeger-span' as the mapping type %w", options.Mapping, err)
	}

	parsedMapping, err := renderer.GetMappingAsString(es.TextTemplateBuilder{}, &options)
	if err != nil {
		return "", fmt.Errorf("failed to render mapping to string: %w", err)
	}

	return parsedMapping, nil
}
