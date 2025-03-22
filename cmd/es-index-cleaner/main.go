// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/featuregate"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/es-index-cleaner/app"
	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/pkg/es/client"
)

var relativeIndexCleaner *featuregate.Gate

func init() {
	relativeIndexCleaner = featuregate.GlobalRegistry().MustRegister(
		"es.index.relativeTimeIndexDeletion",
		featuregate.StageAlpha,
		featuregate.WithRegisterFromVersion("v2.5.0"),
		featuregate.WithRegisterDescription("Controls whether the indices will be deleted relative to the current time or tomorrow midnight."),
		featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/issues/6236"),
	)
}

func main() {
	logger, _ := zap.NewProduction()
	v := viper.New()
	cfg := &app.Config{}

	command := &cobra.Command{
		Use:   "jaeger-es-index-cleaner NUM_OF_DAYS http://HOSTNAME:PORT",
		Short: "Jaeger es-index-cleaner removes Jaeger indices",
		Long:  "Jaeger es-index-cleaner removes Jaeger indices",
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return errors.New("wrong number of arguments")
			}
			numOfDays, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("could not parse NUM_OF_DAYS argument: %w", err)
			}

			cfg.InitFromViper(v)

			ctx := context.Background()
			tlscfg, err := cfg.TLSConfig.LoadTLSConfig(ctx)
			if err != nil {
				return fmt.Errorf("error loading tls config : %w", err)
			}

			c := &http.Client{
				Timeout: time.Duration(cfg.MasterNodeTimeoutSeconds) * time.Second,
				Transport: &http.Transport{
					Proxy:           http.ProxyFromEnvironment,
					TLSClientConfig: tlscfg,
				},
			}
			i := client.IndicesClient{
				Client: client.Client{
					Endpoint:  args[1],
					Client:    c,
					BasicAuth: basicAuth(cfg.Username, cfg.Password),
				},
				MasterTimeoutSeconds:   cfg.MasterNodeTimeoutSeconds,
				IgnoreUnavailableIndex: true,
			}

			indices, err := i.GetJaegerIndices(cfg.IndexPrefix)
			if err != nil {
				return err
			}

			deleteIndicesBefore := app.CalculateDeletionCutoff(time.Now().UTC(), numOfDays, relativeIndexCleaner.IsEnabled())
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
	)

	command.Flags().AddFlagSet(pflag.CommandLine)
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
