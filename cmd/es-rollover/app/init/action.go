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

package init

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/client"
	"github.com/jaegertracing/jaeger/pkg/es/filter"
	"github.com/jaegertracing/jaeger/plugin/storage/es/mappings"
)

const ilmVersionSupport = 7

// Action holds the configuration and clients for init action
type Action struct {
	Config        Config
	ClusterClient client.ClusterClient
	IndicesClient client.IndicesClient
	ILMClient     client.ILMClient
}

func (c Action) getMapping(version uint, templateName string) (string, error) {
	mappingBuilder := mappings.MappingBuilder{
		TemplateBuilder: es.TextTemplateBuilder{},
		Shards:          c.Config.Shards,
		Replicas:        c.Config.Replicas,
		IndexPrefix:     c.Config.IndexPrefix,
		UseILM:          c.Config.UseILM,
		ILMPolicyName:   c.Config.ILMPolicyName,
		EsVersion:       version,
	}
	return mappingBuilder.GetMapping(templateName)
}

// Do the init action
func (c Action) Do() error {
	version, err := c.ClusterClient.Version()
	if err != nil {
		return err
	}
	if c.Config.UseILM {
		if version == ilmVersionSupport {
			policyExist, err := c.ILMClient.Exists(c.Config.ILMPolicyName)
			if err != nil {
				return err
			}
			if !policyExist {
				return fmt.Errorf("ILM policy %s doesn't exist in Elasticsearch. Please create it and re-run init", c.Config.ILMPolicyName)
			}
		} else {
			return fmt.Errorf("ILM is supported only for ES version 7+")
		}
	}
	rolloverIndices := app.RolloverIndices(c.Config.Archive, c.Config.IndexPrefix)
	for _, indexName := range rolloverIndices {
		if err := c.init(version, indexName); err != nil {
			return err
		}
	}

	return nil
}

func (c Action) init(version uint, indexset app.IndexOption) error {
	mapping, err := c.getMapping(version, indexset.TemplateName)
	if err != nil {
		return err
	}

	err = c.IndicesClient.CreateTemplate(mapping, indexset.TemplateName)
	if err != nil {
		return err
	}
	index := indexset.InitialRolloverIndex()

	err = c.IndicesClient.CreateIndex(index)
	if esErr, ok := err.(client.ResponseError); ok {
		if esErr.StatusCode == http.StatusBadRequest && esErr.Body != nil {
			jsonError := map[string]interface{}{}
			err := json.Unmarshal(esErr.Body, &jsonError)
			if err != nil {
				return err
			}
			errorMap := jsonError["error"].(map[string]interface{})
			// Ignore already exist error
			if !strings.Contains("resource_already_exists_exception", errorMap["type"].(string)) {
				return err
			}
		}
	}

	jaegerIndices, err := c.IndicesClient.GetJaegerIndices(c.Config.IndexPrefix)
	if err != nil {
		return err
	}

	readAlias := indexset.ReadAliasName()
	writeAlias := indexset.WriteAliasName()

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
			IsWriteIndex: true,
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
