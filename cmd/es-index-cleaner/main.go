// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/featuregate"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/es-index-cleaner/app"
	"github.com/jaegertracing/jaeger/internal/config"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
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

			if err := cfg.InitFromViper(v); err != nil {
				return fmt.Errorf("failed to initialize config: %w", err)
			}

			ctx := context.Background()
			esClient, err := newESClient(ctx, args[1], cfg, logger)
			if err != nil {
				return fmt.Errorf("error creating Elasticsearch client: %w", err)
			}
			i := esclient.IndicesClient{
				Client:                 esClient,
				MasterTimeoutSeconds:   cfg.MasterNodeTimeoutSeconds,
				IgnoreUnavailableIndex: true,
			}

			indices, err := i.GetJaegerIndices(ctx, cfg.IndexPrefix)
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
			return i.DeleteIndices(ctx, indices)
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

func newESClient(ctx context.Context, endpoint string, cfg *app.Config, logger *zap.Logger) (esclient.Client, error) {
	esCfg := &escfg.Configuration{
		Servers:      []string{endpoint},
		QueryTimeout: time.Duration(cfg.MasterNodeTimeoutSeconds) * time.Second,
		TLS:          cfg.TLSConfig,
	}
	// Enable basic auth only when both are set, matching the prior behavior
	// of omitting the Authorization header unless username and password are present.
	if cfg.Username != "" && cfg.Password != "" {
		esCfg.Authentication.BasicAuthentication = configoptional.Some(escfg.BasicAuthentication{
			Username: cfg.Username,
			Password: cfg.Password,
		})
	}
	if cfg.TokenFilePath != "" {
		esCfg.Authentication.BearerTokenAuth = configoptional.Some(escfg.TokenAuthentication{
			FilePath: cfg.TokenFilePath,
		})
	}
	if cfg.APIKeyFilePath != "" {
		esCfg.Authentication.APIKeyAuth = configoptional.Some(escfg.TokenAuthentication{
			FilePath: cfg.APIKeyFilePath,
		})
	}
	return esclient.NewClient(ctx, esCfg, logger, nil)
}
