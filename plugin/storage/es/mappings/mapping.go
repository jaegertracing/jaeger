// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"bytes"
	"embed"
	"fmt"
	"strings"

	"github.com/jaegertracing/jaeger/pkg/es"
)

// MAPPINGS contains embedded index templates.
//
//go:embed *.json
var MAPPINGS embed.FS

// MappingBuilder holds parameters required to render an elasticsearch index template
type MappingBuilder struct {
	TemplateBuilder              es.TemplateBuilder
	Shards                       int64
	Replicas                     int64
	PrioritySpanTemplate         int64
	PriorityServiceTemplate      int64
	PriorityDependenciesTemplate int64
	PrioritySamplingTemplate     int64
	EsVersion                    uint
	IndexPrefix                  string
	UseILM                       bool
	ILMPolicyName                string
}

// MappingType represents the type of Elasticsearch mapping
type MappingType int

const (
	SpanMapping MappingType = iota
	ServiceMapping
	DependenciesMapping
	SamplingMapping
)

func (mt MappingType) String() (string, error) {
	switch mt {
	case SpanMapping:
		return "jaeger-span", nil
	case ServiceMapping:
		return "jaeger-service", nil
	case DependenciesMapping:
		return "jaeger-dependencies", nil
	case SamplingMapping:
		return "jaeger-sampling", nil
	default:
		return "", fmt.Errorf("Unknown mapping type %d", mt)
	}
}

// GetMapping returns the rendered mapping based on elasticsearch version
func (mb *MappingBuilder) GetMapping(mappingType MappingType) (string, error) {
	var version string
	switch mb.EsVersion {
	case 8:
		version = "-8"
	case 7:
		version = "-7"
	default:
		version = "-6"
	}

	templateName, err := mappingType.String()
	if err != nil {
		return "", err
	}

	return mb.fixMapping(templateName + version + ".json")
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

func (mb *MappingBuilder) fixMapping(mapping string) (string, error) {
	tmpl, err := mb.TemplateBuilder.Parse(loadMapping(mapping))
	if err != nil {
		return "", err
	}
	writer := new(bytes.Buffer)

	if mb.IndexPrefix != "" && !strings.HasSuffix(mb.IndexPrefix, "-") {
		mb.IndexPrefix += "-"
	}
	if err := tmpl.Execute(writer, mb); err != nil {
		return "", err
	}

	return writer.String(), nil
}
