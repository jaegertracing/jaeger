// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"fmt"
	"strconv"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/mappings"
)

// GetMappingAsString returns rendered index templates as string
func GetMappingAsString(builder es.TemplateBuilder, opt *app.Options) (string, error) {
	enableILM, err := strconv.ParseBool(opt.UseILM)
	if err != nil {
		return "", err
	}

	mappingBuilder := mappings.MappingBuilder{
		TemplateBuilder: builder,
		Shards:          opt.Shards,
		Replicas:        opt.Replicas,
		EsVersion:       opt.EsVersion,
		IndexPrefix:     opt.IndexPrefix,
		UseILM:          enableILM,
		ILMPolicyName:   opt.ILMPolicyName,
	}

	mappingType, err := MappingTypeFromString(opt.Mapping)
	if err != nil {
		return "", err
	}

	return mappingBuilder.GetMapping(mappingType)
}

// IsValidOption checks if passed option is a valid index template.
func IsValidOption(val string) bool {
	_, err := MappingTypeFromString(val)
	return err == nil
}

// MappingTypeFromString converts a string to a MappingType
// MappingType represents the type of Elasticsearch mapping
func MappingTypeFromString(val string) (mappings.MappingType, error) {
	switch val {
	case "jaeger-span":
		return mappings.SpanMapping, nil
	case "jaeger-service":
		return mappings.ServiceMapping, nil
	case "jaeger-dependencies":
		return mappings.DependenciesMapping, nil
	case "jaeger-sampling":
		return mappings.SamplingMapping, nil
	default:
		return -1, fmt.Errorf("invalid mapping type: %s", val)
	}
}
