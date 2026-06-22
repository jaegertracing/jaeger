// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"fmt"

	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
)

// componentTemplateCreator creates the composable templates that define a data
// stream. Satisfied by the REST IndicesClient.
type componentTemplateCreator interface {
	CreateComponentTemplate(name, template string) error
	CreateIndexTemplate(name, template string) error
}

// lifecyclePolicyManager creates a lifecycle policy if it does not already
// exist. Satisfied by both the ISMClient (OpenSearch) and the ILMClient
// (Elasticsearch).
type lifecyclePolicyManager interface {
	Exists(name string) (bool, error)
	Create(name, policy string) error
}

// dataStreamBootstrap holds everything needed to create the span data stream's
// templates and lifecycle policy on startup.
type dataStreamBootstrap struct {
	templates      componentTemplateCreator
	lifecycle      lifecyclePolicyManager
	mappingBuilder *mappings.MappingBuilder
	dataStreamName string
	policyName     string
	policyBody     string
	// useILM is true on Elasticsearch (ILM, referenced from index settings) and
	// false on OpenSearch (ISM, self-attaching via the policy's ism_template).
	useILM bool
}

// run creates the lifecycle policy (only if absent, so user customizations are
// never overwritten) and then the @mappings/@settings component templates and the
// composable index template (idempotent overwrites). Component templates are
// created before the index template that composes them. See RFC 0004 sections
// 3.2 and 3.6.
func (b dataStreamBootstrap) run() error {
	exists, err := b.lifecycle.Exists(b.policyName)
	if err != nil {
		return fmt.Errorf("failed to check lifecycle policy %q: %w", b.policyName, err)
	}
	if !exists {
		if err := b.lifecycle.Create(b.policyName, b.policyBody); err != nil {
			return fmt.Errorf("failed to create lifecycle policy %q: %w", b.policyName, err)
		}
	}

	mappingsBody, err := b.mappingBuilder.SpanDataStreamMappings()
	if err != nil {
		return fmt.Errorf("failed to build data stream mappings: %w", err)
	}
	if err := b.templates.CreateComponentTemplate(b.dataStreamName+mappings.DataStreamMappingsSuffix, mappingsBody); err != nil {
		return err
	}

	settingsBody, err := b.mappingBuilder.SpanDataStreamSettings(b.useILM, b.policyName)
	if err != nil {
		return fmt.Errorf("failed to build data stream settings: %w", err)
	}
	if err := b.templates.CreateComponentTemplate(b.dataStreamName+mappings.DataStreamSettingsSuffix, settingsBody); err != nil {
		return err
	}

	indexTemplate, err := mappings.SpanDataStreamIndexTemplate(b.dataStreamName)
	if err != nil {
		return fmt.Errorf("failed to build data stream index template: %w", err)
	}
	return b.templates.CreateIndexTemplate(b.dataStreamName, indexTemplate)
}
