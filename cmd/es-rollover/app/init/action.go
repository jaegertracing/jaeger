// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package init

import (
	"errors"
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/internal/storage/es"
	"github.com/jaegertracing/jaeger/internal/storage/es/client"
	"github.com/jaegertracing/jaeger/internal/storage/es/config"
	"github.com/jaegertracing/jaeger/internal/storage/es/filter"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
)

const ilmVersionSupport = 7

// Action holds the configuration and clients for init action
type Action struct {
	Config        Config
	ClusterClient client.ClusterAPI
	IndicesClient client.IndexAPI
	ILMClient     client.IndexManagementLifecycleAPI
}

func (c Action) getMapping(version uint, mappingType mappings.MappingType) (string, error) {
	c.Config.Indices.IndexPrefix = config.IndexPrefix(c.Config.Config.IndexPrefix)
	mappingBuilder := mappings.MappingBuilder{
		TemplateBuilder: es.TextTemplateBuilder{},
		Indices:         c.Config.Indices,
		UseILM:          c.Config.UseILM,
		ILMPolicyName:   c.Config.ILMPolicyName,
		EsVersion:       version,
	}

	return mappingBuilder.GetMapping(mappingType)
}

// Do the init action
func (c Action) Do() error {
	version, err := c.ClusterClient.Version()
	if err != nil {
		return err
	}
	if c.Config.UseILM {
		if version < ilmVersionSupport {
			return errors.New("ILM is supported only for ES version 7+")
		}
		policyExist, err := c.ILMClient.Exists(c.Config.ILMPolicyName)
		if err != nil {
			return err
		}
		if !policyExist {
			return fmt.Errorf("ILM policy %s doesn't exist in Elasticsearch. Please create it and re-run init", c.Config.ILMPolicyName)
		}
	}
	rolloverIndices := app.RolloverIndices(c.Config.Archive, c.Config.SkipDependencies, c.Config.AdaptiveSampling, c.Config.Config.IndexPrefix)
	for _, indexName := range rolloverIndices {
		if err := c.init(version, indexName); err != nil {
			return err
		}
	}
	return nil
}

func createIndexIfNotExist(c client.IndexAPI, index string) error {
	exists, err := c.IndexExists(index)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	aliasExists, err := c.AliasExists(index)
	if err != nil {
		return err
	}
	if aliasExists {
		return nil
	}
	return c.CreateIndex(index)
}

func (c Action) init(version uint, indexopt app.IndexOption) error {
	mappingType, err := mappings.MappingTypeFromString(indexopt.Mapping)
	if err != nil {
		return err
	}

	mapping, err := c.getMapping(version, mappingType)
	if err != nil {
		return err
	}

	err = c.IndicesClient.CreateTemplate(mapping, indexopt.TemplateName())
	if err != nil {
		return err
	}

	index := indexopt.InitialRolloverIndex()
	err = createIndexIfNotExist(c.IndicesClient, index)
	if err != nil {
		return err
	}

	jaegerIndices, err := c.IndicesClient.GetJaegerIndices(c.Config.Config.IndexPrefix)
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
		err = c.IndicesClient.CreateAlias(aliases)
		if err != nil {
			return err
		}
	}
	return nil
}
