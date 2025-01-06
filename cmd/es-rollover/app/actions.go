// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/es/client"
)

func NewESClient(endpoint string, cfg *Config, tlsCfg *tls.Config) client.Client {
	httpClient := &http.Client{
		Timeout: time.Duration(cfg.Timeout) * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: tlsCfg,
		},
	}
	return client.Client{
		Endpoint:  endpoint,
		Client:    httpClient,
		BasicAuth: client.BasicAuth(cfg.Username, cfg.Password),
	}
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
type ActionCreatorFunction func(client.Client, Config) Action

// ExecuteAction execute the action returned by the createAction function
func ExecuteAction(opts ActionExecuteOptions, createAction ActionCreatorFunction) error {
	cfg := Config{}
	cfg.InitFromViper(opts.Viper)

	ctx := context.Background()
	tlsCfg, err := cfg.TLSConfig.LoadTLSConfig(ctx)
	if err != nil {
		return fmt.Errorf("TLS configuration failed: %w", err)
	}

	esClient := NewESClient(opts.Args[0], &cfg, tlsCfg)
	action := createAction(esClient, cfg)
	return action.Do()
}
