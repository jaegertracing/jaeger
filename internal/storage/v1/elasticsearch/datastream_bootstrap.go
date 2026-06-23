// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/client"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
)

// defaultDataStreamPolicyName is the lifecycle policy name used when the
// data_stream rotation does not specify one. See RFC 0004 section 3.6 (Q5).
const defaultDataStreamPolicyName = "jaeger-spans-policy"

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

	indexTemplate, err := mappings.SpanDataStreamIndexTemplate(b.dataStreamName, !b.useILM)
	if err != nil {
		return fmt.Errorf("failed to build data stream index template: %w", err)
	}
	return b.templates.CreateIndexTemplate(b.dataStreamName, indexTemplate)
}

// bootstrapSpanDataStream creates the composable templates and lifecycle policy
// for the spans data stream. It builds a direct REST client from the same config
// as the main client, detects the backend flavor to choose ISM (OpenSearch) or
// ILM (Elasticsearch), and runs the idempotent bootstrap.
func (f *FactoryBase) bootstrapSpanDataStream(ctx context.Context, mb *mappings.MappingBuilder) error {
	spanPrefix := f.config.Indices.IndexPrefix.Apply(indices.SpanIndexBaseName)
	spanRC := f.config.ResolvedSpanRotation(spanPrefix)
	ds := spanRC.DataStream.Get()

	transport, err := config.GetHTTPRoundTripper(ctx, f.config, f.logger, f.httpAuth)
	if err != nil {
		return fmt.Errorf("failed to build HTTP transport for data stream bootstrap: %w", err)
	}
	rawClient := client.Client{
		Client:   &http.Client{Transport: transport},
		Endpoint: strings.TrimSuffix(f.config.Servers[0], "/"),
	}

	clusterClient := client.ClusterClient{Client: rawClient}
	isOpenSearch, err := clusterClient.IsOpenSearch(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect Elasticsearch/OpenSearch flavor: %w", err)
	}

	dataStreamName := indices.DataStreamName(string(f.config.Indices.IndexPrefix), indices.SpanDataStreamBaseName)
	policyName := ds.PolicyName
	if policyName == "" {
		policyName = defaultDataStreamPolicyName
	}
	policyBody, err := dataStreamPolicyBody(ds, isOpenSearch, dataStreamName, mb)
	if err != nil {
		return err
	}

	var lifecycle lifecyclePolicyManager
	if isOpenSearch {
		lifecycle = client.ISMClient{Client: rawClient, Logger: f.logger}
	} else {
		lifecycle = client.ILMClient{Client: rawClient, Logger: f.logger}
	}

	return dataStreamBootstrap{
		templates:      client.IndicesClient{Client: rawClient},
		lifecycle:      lifecycle,
		mappingBuilder: mb,
		dataStreamName: dataStreamName,
		policyName:     policyName,
		policyBody:     policyBody,
		useILM:         !isOpenSearch,
	}.run()
}

// dataStreamPolicyBody returns the lifecycle policy to install: a user-provided
// file when configured, otherwise the built-in ISM (OpenSearch) or ILM
// (Elasticsearch) default. See RFC 0004 section 3.6.
func dataStreamPolicyBody(ds *config.DataStreamRotation, isOpenSearch bool, dataStreamName string, mb *mappings.MappingBuilder) (string, error) {
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
