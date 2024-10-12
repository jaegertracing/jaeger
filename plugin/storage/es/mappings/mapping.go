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

// templateParams holds parameters required to render an elasticsearch index template
type templateParams struct {
	UseILM        bool
	ILMPolicyName string
	IndexPrefix   string
	Shards        int64
	Replicas      int64
	Priority      int64
}

func (mb MappingBuilder) getMappingTemplateOptions(mapping string) templateParams {
	mappingOpts := templateParams{}
	mappingOpts.UseILM = mb.UseILM
	mappingOpts.ILMPolicyName = mb.ILMPolicyName

	switch {
	case strings.Contains(mapping, "span"):
		mappingOpts.Shards = mb.Indices.Spans.Shards
		mappingOpts.Replicas = mb.Indices.Spans.Replicas
		mappingOpts.Priority = mb.Indices.Spans.Priority
	case strings.Contains(mapping, "service"):
		mappingOpts.Shards = mb.Indices.Services.Shards
		mappingOpts.Replicas = mb.Indices.Services.Replicas
		mappingOpts.Priority = mb.Indices.Services.Priority
	case strings.Contains(mapping, "dependencies"):
		mappingOpts.Shards = mb.Indices.Dependencies.Shards
		mappingOpts.Replicas = mb.Indices.Dependencies.Replicas
		mappingOpts.Priority = mb.Indices.Dependencies.Priority
	case strings.Contains(mapping, "sampling"):
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
		return mb.renderMapping(mapping+"-8.json", templateOpts)
	} else if mb.EsVersion == 7 {
		return mb.renderMapping(mapping+"-7.json", templateOpts)
	}
	return mb.renderMapping(mapping+"-6.json", templateOpts)
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
