// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

// newESClient builds the client and detects the backend version once, so all
// version-dependent operations (template endpoint, ILM vs ISM) are resolved at
// construction time instead of re-detecting per call.
func newESClient(ctx context.Context, endpoint string, cfg *Config, logger *zap.Logger) (*esclient.Client, error) {
	esCfg := &config.Configuration{
		Servers:      []string{endpoint},
		QueryTimeout: time.Duration(cfg.Timeout) * time.Second,
		TLS:          cfg.TLSConfig,
	}
	// Enable basic auth only when both are set, matching the prior behavior of
	// omitting the Authorization header unless username and password are present.
	if cfg.Username != "" && cfg.Password != "" {
		esCfg.Authentication.BasicAuthentication = configoptional.Some(config.BasicAuthentication{
			Username: cfg.Username,
			Password: cfg.Password,
		})
	}
	if cfg.TokenFilePath != "" {
		esCfg.Authentication.BearerTokenAuth = configoptional.Some(config.TokenAuthentication{
			FilePath: cfg.TokenFilePath,
		})
	}
	if cfg.APIKeyFilePath != "" {
		esCfg.Authentication.APIKeyAuth = configoptional.Some(config.TokenAuthentication{
			FilePath: cfg.APIKeyFilePath,
		})
	}
	if err := esCfg.Validate(); err != nil {
		return esclient.Client{}, err
	}
	// NewClient resolves the backend version at construction.
	return esclient.NewClient(ctx, esCfg, logger, nil)
}

// Action is an interface that each action (init, rollover and lookback) of the es-rollover should implement
type Action interface {
	Do() error
}

// ActionExecuteOptions are the options passed to the execute action function
type ActionExecuteOptions struct {
	Args   []string
	Viper  *viper.Viper
	Logger *zap.Logger
}

// ActionCreatorFunction type is the function type in charge of create the action to be executed
type ActionCreatorFunction func(*esclient.Client, Config) Action

// ExecuteAction execute the action returned by the createAction function
func ExecuteAction(opts ActionExecuteOptions, createAction ActionCreatorFunction) error {
	cfg := Config{}
	if err := cfg.InitFromViper(opts.Viper); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	esClient, err := newESClient(context.Background(), opts.Args[0], &cfg, opts.Logger)
	if err != nil {
		return fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}
	action := createAction(esClient, cfg)
	return action.Do()
}
