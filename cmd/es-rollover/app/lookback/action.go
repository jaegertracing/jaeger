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

package lookback

import (
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/pkg/es/client"
	"github.com/jaegertracing/jaeger/pkg/es/filter"
)

var timeNow func() time.Time = time.Now

// Action holds the configuration and clients for lookback action
type Action struct {
	Config
	IndicesClient client.IndexAPI
	Logger        *zap.Logger
}

// Do the lookback action
func (a *Action) Do() error {
	rolloverIndices := app.RolloverIndices(a.Config.Archive, a.Config.SkipDependencies, a.Config.AdaptiveSampling, a.Config.IndexPrefix)
	for _, indexName := range rolloverIndices {
		if err := a.lookback(indexName); err != nil {
			return err
		}
	}
	return nil
}

func (a *Action) lookback(indexSet app.IndexOption) error {
	jaegerIndex, err := a.IndicesClient.GetJaegerIndices(a.Config.IndexPrefix)
	if err != nil {
		return err
	}

	readAliasName := indexSet.ReadAliasName()
	readAliasIndices := filter.ByAlias(jaegerIndex, []string{readAliasName})
	excludedWriteIndex := filter.ByAliasExclude(readAliasIndices, []string{indexSet.WriteAliasName()})
	finalIndices := filter.ByDate(excludedWriteIndex, getTimeReference(timeNow(), a.Unit, a.UnitCount))

	if len(finalIndices) == 0 {
		a.Logger.Info("No indices to remove from alias", zap.String("readAliasName", readAliasName))
		return nil
	}

	aliases := make([]client.Alias, 0, len(finalIndices))
	a.Logger.Info("About to remove indices", zap.String("readAliasName", readAliasName), zap.Int("indicesCount", len(finalIndices)))

	for _, index := range finalIndices {
		aliases = append(aliases, client.Alias{
			Index: index.Index,
			Name:  readAliasName,
		})
		a.Logger.Info("To be removed", zap.String("index", index.Index), zap.String("creationTime", index.CreationTime.String()))
	}

	return a.IndicesClient.DeleteAlias(aliases)
}
