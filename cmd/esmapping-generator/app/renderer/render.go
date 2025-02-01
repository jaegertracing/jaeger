// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"strconv"

	"github.com/jaegertracing/jaeger/cmd/esmapping-generator/app"
	"github.com/jaegertracing/jaeger/internal/storage/v1/es/mappings"
	"github.com/jaegertracing/jaeger/pkg/es"
	cfg "github.com/jaegertracing/jaeger/pkg/es/config"
)

// GetMappingAsString returns rendered index templates as string
func GetMappingAsString(builder es.TemplateBuilder, opt *app.Options) (string, error) {
	enableILM, err := strconv.ParseBool(opt.UseILM)
	if err != nil {
		return "", err
	}

	indexOpts := cfg.IndexOptions{
		Shards:   opt.Shards,
		Replicas: opt.Replicas,
	}
	mappingBuilder := mappings.MappingBuilder{
		TemplateBuilder: builder,
		Indices: cfg.Indices{
			IndexPrefix:  cfg.IndexPrefix(opt.IndexPrefix),
			Spans:        indexOpts,
			Services:     indexOpts,
			Dependencies: indexOpts,
			Sampling:     indexOpts,
		},
		EsVersion:     opt.EsVersion,
		UseILM:        enableILM,
		ILMPolicyName: opt.ILMPolicyName,
	}

	mappingType, err := mappings.MappingTypeFromString(opt.Mapping)
	if err != nil {
		return "", err
	}

	return mappingBuilder.GetMapping(mappingType)
}
