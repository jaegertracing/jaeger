// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
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

func generateMappings(options Options) (string, error) {
	if _, err := MappingTypeFromString(options.Mapping); err != nil {
		return "", fmt.Errorf("invalid mapping type '%s': please pass either 'jaeger-service' or 'jaeger-span' as the mapping type %w", options.Mapping, err)
	}

	parsedMapping, err := getMappingAsString(es.TextTemplateBuilder{}, options)
	if err != nil {
		return "", fmt.Errorf("failed to render mapping to string: %w", err)
	}

	return parsedMapping, nil
}

// getMappingAsString returns rendered index templates as string
func getMappingAsString(builder es.TemplateBuilder, opt Options) (string, error) {
	enableILM, err := strconv.ParseBool(opt.UseILM)
	if err != nil {
		return "", err
	}

	indexOpts := config.IndexOptions{
		Shards:   opt.Shards,
		Replicas: opt.Replicas,
	}
	mappingBuilder := MappingBuilder{
		TemplateBuilder: builder,
		Indices: config.Indices{
			IndexPrefix:  config.IndexPrefix(opt.IndexPrefix),
			Spans:        indexOpts,
			Services:     indexOpts,
			Dependencies: indexOpts,
			Sampling:     indexOpts,
		},
		EsVersion:     opt.EsVersion,
		UseILM:        enableILM,
		ILMPolicyName: opt.ILMPolicyName,
	}

	mappingType, err := MappingTypeFromString(opt.Mapping)
	if err != nil {
		return "", err
	}

	return mappingBuilder.GetMapping(mappingType)
}
