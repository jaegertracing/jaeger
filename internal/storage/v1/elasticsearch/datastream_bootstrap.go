// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"fmt"
	"os"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
)

// defaultDataStreamPolicyName is the lifecycle policy name used when the
// data_stream rotation does not configure one. See RFC 0004 §3.6 (Q5).
const defaultDataStreamPolicyName = "jaeger-spans-policy"

// bootstrapSpanDataStream creates the composable templates and lifecycle policy
// that back the spans data stream (RFC 0004 §3.2 and §3.6). Every request goes
// through the es.Client, reusing the same authenticated client and backend
// (v7/v8/OpenSearch) dispatch as the read/write path.
//
// It is idempotent: the @mappings/@settings component templates and the
// composable index template are overwritten on every startup so mappings stay
// current with the Jaeger version, while the lifecycle policy is created only
// when absent so user customizations are never overwritten. Component templates
// are created before the index template that composes them.
func (f *FactoryBase) bootstrapSpanDataStream(ctx context.Context, mb *mappings.MappingBuilder) error {
	c := f.getClient()
	isOpenSearch := c.GetVersion().IsOpenSearch()
	dataStreamName := indices.SpanDataStreamName(f.config.Indices.IndexPrefix)
	spanRC := f.config.ResolvedSpanRotation()
	ds := spanRC.DataStream.Get()

	policyName := ds.PolicyName
	if policyName == "" {
		policyName = defaultDataStreamPolicyName
	}
	// Create the lifecycle policy only when absent, so user customizations are
	// never overwritten (RFC 0004 §3.6).
	exists, err := c.LifecyclePolicyExists(ctx, policyName)
	if err != nil {
		return fmt.Errorf("failed to check lifecycle policy %q: %w", policyName, err)
	}
	if !exists {
		policyBody, err := dataStreamPolicyBody(ds, mb, isOpenSearch, dataStreamName)
		if err != nil {
			return err
		}
		if err := c.CreateLifecyclePolicy(ctx, policyName, policyBody); err != nil {
			return fmt.Errorf("failed to create lifecycle policy %q: %w", policyName, err)
		}
	}

	mappingsBody, err := mb.SpanDataStreamMappings()
	if err != nil {
		return fmt.Errorf("failed to build data stream mappings: %w", err)
	}
	if err := c.CreateComponentTemplate(ctx, dataStreamName+mappings.DataStreamMappingsSuffix, mappingsBody); err != nil {
		return err
	}

	// On Elasticsearch the @settings template references the ILM policy; on
	// OpenSearch the ISM policy self-attaches by pattern, so no reference is added.
	settingsBody, err := mb.SpanDataStreamSettings(!isOpenSearch, policyName)
	if err != nil {
		return fmt.Errorf("failed to build data stream settings: %w", err)
	}
	if err := c.CreateComponentTemplate(ctx, dataStreamName+mappings.DataStreamSettingsSuffix, settingsBody); err != nil {
		return err
	}

	indexTemplate, err := mappings.SpanDataStreamIndexTemplate(dataStreamName, isOpenSearch)
	if err != nil {
		return fmt.Errorf("failed to build data stream index template: %w", err)
	}
	return c.CreateComposableIndexTemplate(ctx, dataStreamName, indexTemplate)
}

// dataStreamPolicyBody returns the lifecycle policy body to install: the
// user-provided file when configured, otherwise the built-in ISM (OpenSearch) or
// ILM (Elasticsearch) default. See RFC 0004 §3.6.
func dataStreamPolicyBody(ds *config.DataStreamRotation, mb *mappings.MappingBuilder, isOpenSearch bool, dataStreamName string) (string, error) {
	if ds.PolicyFile != "" {
		body, err := os.ReadFile(ds.PolicyFile)
		if err != nil {
			return "", fmt.Errorf("failed to read data stream policy file %q: %w", ds.PolicyFile, err)
		}
		return string(body), nil
	}
	if isOpenSearch {
		return mb.SpanDataStreamISMPolicy(dataStreamName)
	}
	return mappings.SpanDataStreamILMPolicy(), nil
}
