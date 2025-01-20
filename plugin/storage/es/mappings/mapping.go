// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"bytes"
	"embed"
	"fmt"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/config"
)

// MAPPINGS contains embedded index templates.
//
//go:embed *.json
var MAPPINGS embed.FS

// MappingType represents the type of Elasticsearch mapping
type MappingType int

const (
	SpanMapping MappingType = iota
	ServiceMapping
	DependenciesMapping
	SamplingMapping
)

// MappingBuilder holds common parameters required to render an elasticsearch index template
type MappingBuilder struct {
	TemplateBuilder es.TemplateBuilder
	Indices         config.Indices
	EsVersion       uint
	UseILM          bool
	ILMPolicyName   string
	IsOpenSearch    bool
}

// templateParams holds parameters required to render an elasticsearch index template
type templateParams struct {
	UseILM        bool
	ILMPolicyName string
	IndexPrefix   string
	Shards        int64
	Replicas      int64
	Priority      int64
	IsOpenSearch  bool
}

func (mb MappingBuilder) getMappingTemplateOptions(mappingType MappingType) templateParams {
	mappingOpts := templateParams{}
	mappingOpts.UseILM = mb.UseILM
	mappingOpts.ILMPolicyName = mb.ILMPolicyName
	mappingOpts.IsOpenSearch = mb.IsOpenSearch
	switch mappingType {
	case SpanMapping:
		mappingOpts.Shards = mb.Indices.Spans.Shards
		mappingOpts.Replicas = mb.Indices.Spans.Replicas
		mappingOpts.Priority = mb.Indices.Spans.Priority
	case ServiceMapping:
		mappingOpts.Shards = mb.Indices.Services.Shards
		mappingOpts.Replicas = mb.Indices.Services.Replicas
		mappingOpts.Priority = mb.Indices.Services.Priority
	case DependenciesMapping:
		mappingOpts.Shards = mb.Indices.Dependencies.Shards
		mappingOpts.Replicas = mb.Indices.Dependencies.Replicas
		mappingOpts.Priority = mb.Indices.Dependencies.Priority
	case SamplingMapping:
		mappingOpts.Shards = mb.Indices.Sampling.Shards
		mappingOpts.Replicas = mb.Indices.Sampling.Replicas
		mappingOpts.Priority = mb.Indices.Sampling.Priority
	}

	return mappingOpts
}

func (mt MappingType) String() string {
	switch mt {
	case SpanMapping:
		return "jaeger-span"
	case ServiceMapping:
		return "jaeger-service"
	case DependenciesMapping:
		return "jaeger-dependencies"
	case SamplingMapping:
		return "jaeger-sampling"
	default:
		return "unknown"
	}
}

// MappingTypeFromString converts a string to a MappingType
func MappingTypeFromString(val string) (MappingType, error) {
	switch val {
	case "jaeger-span":
		return SpanMapping, nil
	case "jaeger-service":
		return ServiceMapping, nil
	case "jaeger-dependencies":
		return DependenciesMapping, nil
	case "jaeger-sampling":
		return SamplingMapping, nil
	default:
		return -1, fmt.Errorf("invalid mapping type: %s", val)
	}
}

// GetMapping returns the rendered mapping based on elasticsearch version
func (mb *MappingBuilder) GetMapping(mappingType MappingType) (string, error) {
	templateOpts := mb.getMappingTemplateOptions(mappingType)
	return mb.renderMapping(fmt.Sprintf("%s-%d.json", mappingType.String(), mb.EsVersion), templateOpts)
}

// GetSpanServiceMappings returns span and service mappings
func (mb *MappingBuilder) GetSpanServiceMappings() (spanMapping string, serviceMapping string, err error) {
	spanMapping, err = mb.GetMapping(SpanMapping)
	if err != nil {
		return "", "", err
	}
	serviceMapping, err = mb.GetMapping(ServiceMapping)
	if err != nil {
		return "", "", err
	}
	return spanMapping, serviceMapping, nil
}

// GetDependenciesMappings returns dependencies mappings
func (mb *MappingBuilder) GetDependenciesMappings() (string, error) {
	return mb.GetMapping(DependenciesMapping)
}

// GetSamplingMappings returns sampling mappings
func (mb *MappingBuilder) GetSamplingMappings() (string, error) {
	return mb.GetMapping(SamplingMapping)
}

func loadMapping(name string) string {
	s, _ := MAPPINGS.ReadFile(name)
	return string(s)
}

func (mb *MappingBuilder) renderMapping(mapping string, options templateParams) (string, error) {
	tmpl, err := mb.TemplateBuilder.Parse(loadMapping(mapping))
	if err != nil {
		return "", err
	}
	writer := new(bytes.Buffer)

	options.IndexPrefix = mb.Indices.IndexPrefix.Apply("")
	if err := tmpl.Execute(writer, options); err != nil {
		return "", err
	}

	return writer.String(), nil
}
