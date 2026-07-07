// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

func Command() *cobra.Command {
	options := Options{}
	command := &cobra.Command{
		Use:   "elasticsearch-mappings",
		Short: "Jaeger esmapping-generator prints rendered mappings as string",
		Long:  "Jaeger esmapping-generator renders passed templates with provided values and prints rendered output to stdout",
		RunE: func(_ *cobra.Command, _ /* args */ []string) error {
			result, err := generateMappings(options)
			if err != nil {
				return fmt.Errorf("error generating mappings: %w", err)
			}
			fmt.Println(result)
			return nil
		},
	}
	options.AddFlags(command)

	return command
}

// generateMappings renders the index template for the requested mapping type and
// backend version. It is an offline generator, so it renders through
// esclient.RenderIndexTemplate for an explicitly-passed version rather than a
// version resolved from a live cluster.
func generateMappings(options Options) (string, error) {
	mappingType, err := esclient.MappingTypeFromString(options.Mapping)
	if err != nil {
		return "", fmt.Errorf("invalid mapping type %q: please pass one of %q, %q, %q, or %q as the mapping type: %w", options.Mapping, config.SpanIndexName, config.ServiceIndexName, config.DependencyIndexName, config.SamplingIndexName, err)
	}
	enableILM, err := strconv.ParseBool(options.UseILM)
	if err != nil {
		return "", err
	}
	indexOpts := config.IndexOptions{
		Shards:   options.Shards,
		Replicas: options.Replicas,
	}
	indices := config.Indices{
		IndexPrefix:  config.IndexPrefix(options.IndexPrefix),
		Spans:        indexOpts,
		Services:     indexOpts,
		Dependencies: indexOpts,
		Sampling:     indexOpts,
	}
	rendered, err := esclient.RenderIndexTemplate(mappingType, indices, enableILM, options.ILMPolicyName, options.BackendVersion())
	if err != nil {
		return "", fmt.Errorf("failed to render mapping to string: %w", err)
	}
	return rendered, nil
}
