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
	"fmt"
	"time"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/pkg/es/client"
	"github.com/jaegertracing/jaeger/pkg/es/filter"
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

	jaegerIndicex, err := a.IndicesClient.GetJaegerIndices(a.Config.IndexPrefix)
	if err != nil {
		return err
	}

	readAliasName := indexSet.ReadAliasName()
	readAliasIndices := filter.ByAlias(jaegerIndicex, []string{readAliasName})
	excludedWriteIndex := filter.ByAliasExclude(readAliasIndices, []string{indexSet.WriteAliasName()})
	finalIndices := filter.ByDate(excludedWriteIndex, getTimeReference(time.Now(), a.Unit, a.UnitCount))
	if len(finalIndices) == 0 {
		return fmt.Errorf("no indices to remove from alias %s", readAliasName)
	}

	aliases := make([]client.Alias, 0, len(finalIndices))

	for _, index := range finalIndices {
		aliases = append(aliases, client.Alias{
			Index: index.Index,
			Name:  readAliasName,
		})
	}
	return a.IndicesClient.DeleteAlias(aliases)

}

func getTimeReference(now time.Time, units string, unitCount int) time.Time {
	switch units {
	case "minutes":
		return now.Truncate(time.Minute).Add(time.Minute).Add(-time.Duration(unitCount) * time.Minute)
	case "hours":
		return now.Truncate(time.Minute).Add(time.Hour).Add(-time.Duration(unitCount) * time.Hour)
	case "days":
		year, month, day := time.Now().Date()
		return time.Date(year, month, day, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -1*unitCount)
	case "weeks":
		diff := int(now.Weekday()) - int(time.Saturday)
		year, month, day := time.Now().Date()
		weekEnd := time.Date(year, month, day, 0, 0, 0, 0, now.Location()).AddDate(0, 0, diff)
		return weekEnd.Add(-time.Hour * 24 * 7 * time.Duration(unitCount))
	case "months":
		year, month, day := time.Now().Date()
		return time.Date(year, month, day, 0, 0, 0, 0, now.Location()).AddDate(0, -1*unitCount, 0)
	case "years":
		year, month, day := time.Now().Date()
		return time.Date(year, month, day, 0, 0, 0, 0, now.Location()).AddDate(1*unitCount, 0, 0)
	}

	return now.Add(-time.Duration(unitCount) * time.Second)

}
