// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

func newESClient(endpoint string, cfg *Config, tlsCfg *tls.Config) (esclient.Client, error) {
	return esclient.NewClient(
		[]string{endpoint},
		tlsCfg,
		esclient.BasicAuth(cfg.Username, cfg.Password),
		time.Duration(cfg.Timeout)*time.Second,
	)
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
type ActionCreatorFunction func(esclient.Client, Config) Action

// ExecuteAction execute the action returned by the createAction function
func ExecuteAction(opts ActionExecuteOptions, createAction ActionCreatorFunction) error {
	cfg := Config{}
	if err := cfg.InitFromViper(opts.Viper); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	ctx := context.Background()
	tlsCfg, err := cfg.TLSConfig.LoadTLSConfig(ctx)
	if err != nil {
		return fmt.Errorf("TLS configuration failed: %w", err)
	}

	esClient, err := newESClient(opts.Args[0], &cfg, tlsCfg)
	if err != nil {
		return fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}
	action := createAction(esClient, cfg)
	return action.Do()
}
