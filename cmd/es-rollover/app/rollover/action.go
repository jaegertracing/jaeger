// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package rollover

import (
	"encoding/json"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/client"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/filter"
)

// Action holds the configuration and clients for rollover action
type Action struct {
	Config
	IndicesClient client.IndexAPI
}

// Do the rollover action
func (a *Action) Do() error {
	rolloverIndices := app.RolloverIndices(a.Config.Archive, a.Config.SkipDependencies, a.Config.AdaptiveSampling, a.Config.IndexPrefix)
	for _, indexName := range rolloverIndices {
		if err := a.rollover(indexName); err != nil {
			return err
		}
	}
	return nil
}

func (a *Action) rollover(indexSet app.IndexOption) error {
	conditionsMap := map[string]any{}
	if len(a.Conditions) > 0 {
		err := json.Unmarshal([]byte(a.Config.Conditions), &conditionsMap)
		if err != nil {
			return err
		}
	}

	writeAlias := indexSet.WriteAliasName()
	readAlias := indexSet.ReadAliasName()
	err := a.IndicesClient.Rollover(writeAlias, conditionsMap)
	if err != nil {
		return err
	}
	jaegerIndex, err := a.IndicesClient.GetJaegerIndices(a.Config.IndexPrefix)
	if err != nil {
		return err
	}

	indicesWithWriteAlias := filter.ByAlias(jaegerIndex, []string{writeAlias})
	aliases := make([]client.Alias, 0, len(indicesWithWriteAlias))
	for _, index := range indicesWithWriteAlias {
		aliases = append(aliases, client.Alias{
			Index: index.Index,
			Name:  readAlias,
		})
	}
	if len(aliases) == 0 {
		return nil
	}
	return a.IndicesClient.CreateAlias(aliases)
}
