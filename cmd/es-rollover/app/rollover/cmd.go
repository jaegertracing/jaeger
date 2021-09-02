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
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/es/client"
)

func Command(v *viper.Viper, logger *zap.Logger) *cobra.Command {
	cfg := &Config{}
	tlsFlags := tlscfg.ClientFlagsConfig{Prefix: "es"}
	command := &cobra.Command{
		Use:   "rollover http://HOSTNAME:PORT",
		Short: "rollover to new write index",
		Long:  "rollover to new write index",
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

			rolloverAction := RolloverAction{
				IndicesClient: indicesClient,
				Config:        *cfg,
			}
			return rolloverAction.Do()
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

type RolloverAction struct {
	Config
	IndicesClient client.IndicesClient
}

func (a *RolloverAction) Do() error {
	rolloverIndices := app.RolloverIndices(a.Config.Archive, a.Config.IndexPrefix)
	for _, indexName := range rolloverIndices {
		if err := a.action(indexName); err != nil {
			return err
		}
	}
	return nil
}

func (a *RolloverAction) action(indexSet app.IndexSet) error {
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
