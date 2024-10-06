// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"bytes"
	"embed"
	"strings"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/config"
)

// MAPPINGS contains embedded index templates.
//
//go:embed *.json
var MAPPINGS embed.FS

// MappingBuilder holds common parameters required to render an elasticsearch index template
type MappingBuilder struct {
	TemplateBuilder es.TemplateBuilder
	Indices         config.Indices
	EsVersion       uint
	UseILM          bool
	ILMPolicyName   string
}

// IndexTemplateOptions holds parameters required to render an elasticsearch index template
type IndexTemplateOptions struct {
	UseILM        bool
	ILMPolicyName string
	Prefix        string
	Shards        int
	Replicas      int
	Priority      int
}

func (mb *MappingBuilder) getMappingTemplateOptions(mapping string) *IndexTemplateOptions {
	mappingOpts := &IndexTemplateOptions{}
	mappingOpts.UseILM = mb.UseILM
	mappingOpts.ILMPolicyName = mb.ILMPolicyName

	switch {
	case strings.Contains(mapping, "span"):
		mappingOpts.Prefix = mb.Indices.Spans.Prefix
		mappingOpts.Shards = mb.Indices.Spans.Shards
		mappingOpts.Replicas = mb.Indices.Spans.Replicas
		mappingOpts.Priority = mb.Indices.Spans.Priority
	case strings.Contains(mapping, "service"):
		mappingOpts.Prefix = mb.Indices.Services.Prefix
		mappingOpts.Shards = mb.Indices.Services.Shards
		mappingOpts.Replicas = mb.Indices.Services.Replicas
		mappingOpts.Priority = mb.Indices.Services.Priority
	case strings.Contains(mapping, "dependencies"):
		mappingOpts.Prefix = mb.Indices.Dependencies.Prefix
		mappingOpts.Shards = mb.Indices.Dependencies.Shards
		mappingOpts.Replicas = mb.Indices.Dependencies.Replicas
		mappingOpts.Priority = mb.Indices.Dependencies.Priority
	case strings.Contains(mapping, "sampling"):
		mappingOpts.Prefix = mb.Indices.Sampling.Prefix
		mappingOpts.Shards = mb.Indices.Sampling.Shards
		mappingOpts.Replicas = mb.Indices.Sampling.Replicas
		mappingOpts.Priority = mb.Indices.Sampling.Priority
	}

	return mappingOpts
}

// GetMapping returns the rendered mapping based on elasticsearch version
func (mb *MappingBuilder) GetMapping(mapping string) (string, error) {
	templateOpts := mb.getMappingTemplateOptions(mapping)
	if mb.EsVersion == 8 {
		return mb.fixMapping(mapping+"-8.json", templateOpts)
	} else if mb.EsVersion == 7 {
		return mb.fixMapping(mapping+"-7.json", templateOpts)
	}
	return mb.fixMapping(mapping+"-6.json", templateOpts)
}

// GetSpanServiceMappings returns span and service mappings
func (mb *MappingBuilder) GetSpanServiceMappings() (spanMapping string, serviceMapping string, err error) {
	spanMapping, err = mb.GetMapping("jaeger-span")
	if err != nil {
		return "", "", err
	}
	serviceMapping, err = mb.GetMapping("jaeger-service")
	if err != nil {
		return "", "", err
	}
	return spanMapping, serviceMapping, nil
}

// GetDependenciesMappings returns dependencies mappings
func (mb *MappingBuilder) GetDependenciesMappings() (string, error) {
	return mb.GetMapping("jaeger-dependencies")
}

// GetSamplingMappings returns sampling mappings
func (mb *MappingBuilder) GetSamplingMappings() (string, error) {
	return mb.GetMapping("jaeger-sampling")
}

func loadMapping(name string) string {
	s, _ := MAPPINGS.ReadFile(name)
	return string(s)
}

func (mb *MappingBuilder) fixMapping(mapping string, options *IndexTemplateOptions) (string, error) {
	tmpl, err := mb.TemplateBuilder.Parse(loadMapping(mapping))
	if err != nil {
		return "", err
	}
	writer := new(bytes.Buffer)

	if options.Prefix != "" && !strings.HasSuffix(options.Prefix, "-") {
		options.Prefix += "-"
	}
	if err := tmpl.Execute(writer, options); err != nil {
		return "", err
	}

	return writer.String(), nil
}
