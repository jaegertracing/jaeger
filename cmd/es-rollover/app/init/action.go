// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package init

import (
	"context"
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/client"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/filter"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
)

// Action holds the configuration and clients for init action
type Action struct {
	Config        Config
	ClusterClient client.ClusterAPI
	IndicesClient client.IndexAPI
	ILMClient     client.IndexManagementLifecycleAPI
}

func (c Action) mappingBuilder(version es.BackendVersion) mappings.MappingBuilder {
	c.Config.Indices.IndexPrefix = config.IndexPrefix(c.Config.Config.IndexPrefix)
	return mappings.MappingBuilder{
		TemplateBuilder: es.TextTemplateBuilder{},
		Indices:         c.Config.Indices,
		UseILM:          c.Config.UseILM,
		ILMPolicyName:   c.Config.ILMPolicyName,
		Version:         version,
	}
}

func (c Action) getMapping(version es.BackendVersion, mappingType mappings.MappingType) (string, error) {
	mappingBuilder := c.mappingBuilder(version)
	return mappingBuilder.GetMapping(mappingType)
}

// createSpanSettingsComponentTemplate creates the @settings component template
// that the composable (v8) span index template references in composed_of
// (RFC 0004 §3.2). The collector creates the same component template at
// startup; the PUT is idempotent.
func (c Action) createSpanSettingsComponentTemplate(ctx context.Context, version es.BackendVersion) error {
	mappingBuilder := c.mappingBuilder(version)
	body, err := mappingBuilder.GetSpanSettingsComponentTemplate()
	if err != nil {
		return err
	}
	name := indices.SpanDataStreamName(config.IndexPrefix(c.Config.Config.IndexPrefix)) + mappings.ComponentTemplateSettingsSuffix
	return c.IndicesClient.CreateComponentTemplate(ctx, body, name)
}

// Do the init action
func (c Action) Do() error {
	ctx := context.TODO()
	version, err := c.ClusterClient.Version(ctx)
	if err != nil {
		return err
	}
	if c.Config.UseILM {
		if !version.SupportsILM() {
			return fmt.Errorf("ILM/ISM is not supported in %s", version)
		}
		if ilm, ok := c.ILMClient.(*client.ILMClient); ok && version.IsOpenSearch() {
			ilm.UseOpenSearchISM = true
		}
		policyExist, err := c.ILMClient.Exists(ctx, c.Config.ILMPolicyName)
		if err != nil {
			return err
		}
		if !policyExist {
			return fmt.Errorf("ILM policy %s doesn't exist in Elasticsearch. Please create it and re-run init", c.Config.ILMPolicyName)
		}
	}
	rolloverIndices := app.RolloverIndices(c.Config.Archive, c.Config.SkipDependencies, c.Config.AdaptiveSampling, c.Config.Config.IndexPrefix)
	for _, indexName := range rolloverIndices {
		if err := c.init(ctx, version, indexName); err != nil {
			return err
		}
	}
	return nil
}

func createIndexIfNotExist(ctx context.Context, c client.IndexAPI, index string) error {
	exists, err := c.IndexExists(ctx, index)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	aliasExists, err := c.AliasExists(ctx, index)
	if err != nil {
		return err
	}
	if aliasExists {
		return nil
	}
	return c.CreateIndex(ctx, index)
}

func (c Action) init(ctx context.Context, version es.BackendVersion, indexopt app.IndexOption) error {
	mappingType, err := mappings.MappingTypeFromString(indexopt.Mapping)
	if err != nil {
		return err
	}

	mapping, err := c.getMapping(version, mappingType)
	if err != nil {
		return err
	}

	// The composable (v8) span template references the spans @settings component
	// template in composed_of, so the component must exist before the template.
	if mappingType == mappings.SpanMapping && version.UsesV8API() {
		if err := c.createSpanSettingsComponentTemplate(ctx, version); err != nil {
			return err
		}
	}

	err = c.IndicesClient.CreateTemplate(ctx, mapping, indexopt.TemplateName())
	if err != nil {
		return err
	}

	index := indexopt.InitialRolloverIndex()
	err = createIndexIfNotExist(ctx, c.IndicesClient, index)
	if err != nil {
		return err
	}

	jaegerIndices, err := c.IndicesClient.GetJaegerIndices(ctx, c.Config.Config.IndexPrefix)
	if err != nil {
		return err
	}

	readAlias := indexopt.ReadAliasName()
	writeAlias := indexopt.WriteAliasName()
	aliases := []client.Alias{}

	if !filter.AliasExists(jaegerIndices, readAlias) {
		aliases = append(aliases, client.Alias{
			Index:        index,
			Name:         readAlias,
			IsWriteIndex: false,
		})
	}

	if !filter.AliasExists(jaegerIndices, writeAlias) {
		aliases = append(aliases, client.Alias{
			Index:        index,
			Name:         writeAlias,
			IsWriteIndex: c.Config.UseILM,
		})
	}

	if len(aliases) > 0 {
		err = c.IndicesClient.CreateAlias(ctx, aliases)
		if err != nil {
			return err
		}
	}
	return nil
}
