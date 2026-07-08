// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package init

import (
	"context"
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/filter"
)

// Action holds the configuration and clients for init action
type Action struct {
	Config        Config
	IndicesClient esclient.IndexAPI
	ILMClient     esclient.IndexManagementLifecycleAPI
}

// Do the init action
func (c Action) Do() error {
	ctx := context.TODO()
	if c.Config.UseILM {
		// Every supported backend provides lifecycle management (ILM on
		// Elasticsearch, ISM on OpenSearch), so no capability check is needed.
		policyExist, err := c.ILMClient.Exists(ctx, c.Config.ILMPolicyName)
		if err != nil {
			return err
		}
		if !policyExist {
			return fmt.Errorf("ILM/ISM policy %s doesn't exist. Please create it and re-run init", c.Config.ILMPolicyName)
		}
	}
	rolloverIndices := app.RolloverIndices(c.Config.Archive, c.Config.SkipDependencies, c.Config.AdaptiveSampling, c.Config.Config.IndexPrefix)
	for _, indexName := range rolloverIndices {
		if err := c.init(ctx, indexName); err != nil {
			return err
		}
	}
	return nil
}

func createIndexIfNotExist(ctx context.Context, c esclient.IndexAPI, index string) error {
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

func (c Action) init(ctx context.Context, indexopt app.IndexOption) error {
	mappingType, err := esclient.MappingTypeFromString(indexopt.Mapping)
	if err != nil {
		return err
	}

	// The client renders the mapping body from its own resolved backend version;
	// the action selects the mapping type but never handles a BackendVersion.
	if err := c.IndicesClient.CreateTemplate(ctx, indexopt.TemplateName(), mappingType); err != nil {
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
	aliases := []esclient.Alias{}

	if !filter.AliasExists(jaegerIndices, readAlias) {
		aliases = append(aliases, esclient.Alias{
			Index:        index,
			Name:         readAlias,
			IsWriteIndex: false,
		})
	}

	if !filter.AliasExists(jaegerIndices, writeAlias) {
		aliases = append(aliases, esclient.Alias{
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
