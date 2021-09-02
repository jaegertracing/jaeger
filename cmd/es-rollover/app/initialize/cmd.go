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

package initialize

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/client"
	"github.com/jaegertracing/jaeger/plugin/storage/es/mappings"
)

const ilmVersionSupport = 7

func Command(v *viper.Viper, logger *zap.Logger) *cobra.Command {
	cfg := &Config{}
	tlsFlags := tlscfg.ClientFlagsConfig{Prefix: "es"}
	command := &cobra.Command{
		Use:   "init http://HOSTNAME:PORT",
		Short: "creates indices and aliases",
		Long:  "creates indices and aliases",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("wrong number of arguments")
			}

			cfg.InitFromViper(v)
			tlsOpts := tlsFlags.InitFromViper(v)
			tlsCfg, err := tlsOpts.Config(logger)
			if err != nil {
				return err
			}
			defer tlsOpts.Close()

			httpClient := &http.Client{
				Timeout: time.Duration(cfg.Timeout) * time.Second,
				Transport: &http.Transport{
					Proxy:           http.ProxyFromEnvironment,
					TLSClientConfig: tlsCfg,
				},
			}
			esClient := client.Client{
				Endpoint:  args[0],
				Client:    httpClient,
				BasicAuth: app.BasicAuth(cfg.Username, cfg.Password),
			}

			indicesClient := client.IndicesClient{
				Client:               esClient,
				MasterTimeoutSeconds: cfg.Timeout,
			}
			clusterClient := client.ClusterClient{
				Client: esClient,
			}

			initCommand := InitCommand{
				IndicesClient: indicesClient,
				ClusterClient: clusterClient,
				Config:        *cfg,
			}
			return initCommand.Do()
		},
	}
	config.AddFlags(
		v,
		command,
		cfg.AddFlags,
		tlsFlags.AddFlags,
	)
	return command
}

type InitCommand struct {
	Config        Config
	ClusterClient client.ClusterClient
	IndicesClient client.IndicesClient
}

func (c InitCommand) getMapping(version uint, templateName string) (string, error) {
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

func (c InitCommand) Do() error {
	version, err := c.ClusterClient.Version()
	if err != nil {
		return err
	}
	if c.Config.UseILM {
		if version == ilmVersionSupport {
			if err := c.IndicesClient.ILMPolicyExists(c.Config.ILMPolicyName); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("ILM is supported only for ES version 7+")
		}
	}
	rolloverIndices := app.RolloverIndices(c.Config.Archive, c.Config.IndexPrefix)
	for _, indexName := range rolloverIndices {
		if err := c.action(version, indexName); err != nil {
			return err
		}
	}

	return nil
}

func (c InitCommand) action(version uint, indexset app.IndexSet) error {
	mapping, err := c.getMapping(version, indexset.Template)
	if err != nil {
		return err
	}

	err = c.IndicesClient.CreateTemplate(mapping, indexset.Template)
	if err != nil {
		return err
	}
	index := indexset.InitialRolloverIndex()

	err = c.IndicesClient.Create(index)
	if esErr, ok := err.(client.ResponseError); ok {
		if esErr.StatusCode == http.StatusBadRequest && esErr.Body != nil {
			jsonError := map[string]interface{}{}
			json.Unmarshal(esErr.Body, &jsonError)
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

	aliasFilter := app.AliasFilter{
		Indices: jaegerIndices,
	}

	readAlias := indexset.ReadAliasName()
	writeAlias := indexset.WriteAliasName()

	aliases := []client.Alias{}

	if aliasFilter.HasAliasEmpty(readAlias) {
		aliases = append(aliases, client.Alias{
			Index:        index,
			Name:         readAlias,
			IsWriteIndex: false,
		})
	}

	if aliasFilter.HasAliasEmpty(writeAlias) {
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
