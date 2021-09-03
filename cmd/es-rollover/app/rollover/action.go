// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rollover

import (
	"encoding/json"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/pkg/es/client"
)

type Action struct {
	Config
	IndicesClient client.IndicesClient
}

func (a *Action) Do() error {
	rolloverIndices := app.RolloverIndices(a.Config.Archive, a.Config.IndexPrefix)
	for _, indexName := range rolloverIndices {
		if err := a.action(indexName); err != nil {
			return err
		}
	}
	return nil
}

func (a *Action) action(indexSet app.IndexOptions) error {
	conditionsMap := map[string]interface{}{}
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
	jaegerIndicex, err := a.IndicesClient.GetJaegerIndices(a.Config.IndexPrefix)
	if err != nil {
		return err
	}
	aliasFilter := app.AliasFilter{
		Indices: jaegerIndicex,
	}
	indicesWithWriteAlias := aliasFilter.FilterByAlias([]string{writeAlias})
	aliases := make([]client.Alias, 0, len(indicesWithWriteAlias))
	for _, index := range indicesWithWriteAlias {
		aliases = append(aliases, client.Alias{
			Index: index.Index,
			Name:  readAlias,
		})
	}
	return a.IndicesClient.CreateAlias(aliases)
}
