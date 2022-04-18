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

package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/es-index-cleaner/app"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/es/client"
)

func main() {
	logger, _ := zap.NewProduction()
	v := viper.New()
	cfg := &app.Config{}
	tlsFlags := tlscfg.ClientFlagsConfig{Prefix: "es"}

	var command = &cobra.Command{
		Use:   "jaeger-es-index-cleaner NUM_OF_DAYS http://HOSTNAME:PORT",
		Short: "Jaeger es-index-cleaner removes Jaeger indices",
		Long:  "Jaeger es-index-cleaner removes Jaeger indices",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("wrong number of arguments")
			}
			numOfDays, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("could not parse NUM_OF_DAYS argument: %w", err)
			}

			cfg.InitFromViper(v)
			tlsOpts, err := tlsFlags.InitFromViper(v)
			if err != nil {
				return err
			}
			tlsCfg, err := tlsOpts.Config(logger)
			if err != nil {
				return err
			}
			defer tlsOpts.Close()

			c := &http.Client{
				Timeout: time.Duration(cfg.MasterNodeTimeoutSeconds) * time.Second,
				Transport: &http.Transport{
					Proxy:           http.ProxyFromEnvironment,
					TLSClientConfig: tlsCfg,
				},
			}
			i := client.IndicesClient{
				Client: client.Client{
					Endpoint:  args[1],
					Client:    c,
					BasicAuth: basicAuth(cfg.Username, cfg.Password),
				},
				MasterTimeoutSeconds: cfg.MasterNodeTimeoutSeconds,
			}

			indices, err := i.GetJaegerIndices(cfg.IndexPrefix)
			if err != nil {
				return err
			}

			year, month, day := time.Now().UTC().Date()
			tomorrowMidnight := time.Date(year, month, day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
			deleteIndicesBefore := tomorrowMidnight.Add(-time.Hour * 24 * time.Duration(numOfDays))
			logger.Info("Indices before this date will be deleted", zap.String("date", deleteIndicesBefore.Format(time.RFC3339)))

			filter := &app.IndexFilter{
				IndexPrefix:          cfg.IndexPrefix,
				IndexDateSeparator:   cfg.IndexDateSeparator,
				Archive:              cfg.Archive,
				Rollover:             cfg.Rollover,
				DeleteBeforeThisDate: deleteIndicesBefore,
			}
			logger.Info("Queried indices", zap.Any("indices", indices))
			indices = filter.Filter(indices)

			if len(indices) == 0 {
				logger.Info("No indices to delete")
				return nil
			}
			logger.Info("Deleting indices", zap.Any("indices", indices))
			return i.DeleteIndices(indices)
		},
	}

	config.AddFlags(
		v,
		command,
		cfg.AddFlags,
		tlsFlags.AddFlags,
	)

	if err := command.Execute(); err != nil {
		log.Fatalln(err)
	}
}

func basicAuth(username, password string) string {
	if username == "" || password == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}
