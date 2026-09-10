// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package rollover

import (
	"context"
	"encoding/json"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/filter"
)

// Action holds the configuration and clients for rollover action
type Action struct {
	Config
	IndicesClient esclient.IndexAPI
}

// Do the rollover action
func (a *Action) Do() error {
	ctx := context.TODO()
	rolloverIndices := app.RolloverIndices(a.Config.Archive, a.Config.SkipDependencies, a.Config.AdaptiveSampling, a.Config.IndexPrefix)
	for _, indexName := range rolloverIndices {
		if err := a.rollover(ctx, indexName); err != nil {
			return err
		}
	}
	return nil
}

func (a *Action) rollover(ctx context.Context, indexSet app.IndexOption) error {
	conditionsMap := map[string]any{}
	if a.Conditions != "" {
		err := json.Unmarshal([]byte(a.Config.Conditions), &conditionsMap)
		if err != nil {
			return err
		}
	}

	writeAlias := indexSet.WriteAliasName()
	readAlias := indexSet.ReadAliasName()
	err := a.IndicesClient.Rollover(ctx, writeAlias, conditionsMap)
	if err != nil {
		return err
	}
	jaegerIndex, err := a.IndicesClient.GetJaegerIndices(ctx, a.Config.IndexPrefix)
	if err != nil {
		return err
	}

	indicesWithWriteAlias := filter.ByAlias(jaegerIndex, []string{writeAlias})
	aliases := make([]esclient.Alias, 0, len(indicesWithWriteAlias))
	for _, index := range indicesWithWriteAlias {
		aliases = append(aliases, esclient.Alias{
			Index: index.Index,
			Name:  readAlias,
		})
	}
	if len(aliases) == 0 {
		return nil
	}
	return a.IndicesClient.CreateAlias(ctx, aliases)
}
