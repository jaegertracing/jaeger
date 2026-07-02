// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"bytes"
	"embed"
	"fmt"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
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

// ComponentTemplateSettingsSuffix is appended to the spans data stream name to
// form the settings component-template name, following the "@" naming
// convention (RFC 0004 §3.2), e.g. "jaeger.spans@settings".
const ComponentTemplateSettingsSuffix = "@settings"

// MappingBuilder holds common parameters required to render an elasticsearch index template
type MappingBuilder struct {
	TemplateBuilder es.TemplateBuilder
	Indices         config.Indices
	Version         es.BackendVersion
	UseILM          bool
	ILMPolicyName   string
}

// templateParams holds parameters required to render an elasticsearch index template
type templateParams struct {
	UseILM        bool
	ILMPolicyName string
	IsOpenSearch  bool
	IndexPrefix   string
	// SpanDataStreamName is the prefixed dot-notation spans data stream name
	// (e.g. "prod.jaeger.spans"), used to reference its component templates.
	SpanDataStreamName string
	Shards             int64
	Replicas           int64
	Priority           int64
}

func (mb MappingBuilder) getMappingTemplateOptions(mappingType MappingType) templateParams {
	mappingOpts := templateParams{}
	mappingOpts.UseILM = mb.UseILM
	mappingOpts.ILMPolicyName = mb.ILMPolicyName
	mappingOpts.IsOpenSearch = mb.Version.IsOpenSearch()

	switch mappingType {
	case SpanMapping:
		mappingOpts.Shards = mb.Indices.Spans.Shards
		mappingOpts.Replicas = *mb.Indices.Spans.Replicas
		mappingOpts.Priority = mb.Indices.Spans.Priority
	case ServiceMapping:
		mappingOpts.Shards = mb.Indices.Services.Shards
		mappingOpts.Replicas = *mb.Indices.Services.Replicas
		mappingOpts.Priority = mb.Indices.Services.Priority
	case DependenciesMapping:
		mappingOpts.Shards = mb.Indices.Dependencies.Shards
		mappingOpts.Replicas = *mb.Indices.Dependencies.Replicas
		mappingOpts.Priority = mb.Indices.Dependencies.Priority
	case SamplingMapping:
		mappingOpts.Shards = mb.Indices.Sampling.Shards
		mappingOpts.Replicas = *mb.Indices.Sampling.Replicas
		mappingOpts.Priority = mb.Indices.Sampling.Priority
	default:
		// Using default values as fallback to avoid breaking functionality.
		mappingOpts.Shards = 5
		mappingOpts.Replicas = 1
		mappingOpts.Priority = 0
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
	return mb.renderMapping(fmt.Sprintf("%s-%d.json", mappingType.String(), mb.Version.TemplateVersion()), templateOpts)
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

// GetSpanSettingsComponentTemplate returns the settings component-template body
// for the spans indices (RFC 0004 §3.2): the shard and replica settings shared
// by the composable span index template (which lists it in composed_of) and,
// later, the spans data stream. The body is version-independent: the
// _component_template API uses the same format on every backend that has it.
func (mb *MappingBuilder) GetSpanSettingsComponentTemplate() (string, error) {
	return mb.renderMapping("jaeger-spans-component-settings.json", mb.getMappingTemplateOptions(SpanMapping))
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
	options.SpanDataStreamName = indices.SpanDataStreamName(mb.Indices.IndexPrefix)
	if err := tmpl.Execute(writer, options); err != nil {
		return "", err
	}

	return writer.String(), nil
}
