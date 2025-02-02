// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"fmt"
	"log"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/config"
)

func Command() *cobra.Command {
	options := Options{}
	command := &cobra.Command{
		Use:   "elasticsearch-mappings",
		Short: "Jaeger esmapping-generator prints rendered mappings as string",
		Long:  "Jaeger esmapping-generator renders passed templates with provided values and prints rendered output to stdout",
		Run: func(_ *cobra.Command, _ /* args */ []string) {
			result, err := generateMappings(options)
			if err != nil {
				log.Fatalf("Error generating mappings: %v", err)
			}
			fmt.Println(result)
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
