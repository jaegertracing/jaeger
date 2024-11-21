// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/es/client"
)

func newESClient(endpoint string, cfg *Config, tlsCfg *tls.Config) client.Client {
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

type ClientConfig struct {
	configtls.ClientConfig `mapstructure:",squash"`
	Enabled                bool
}

// ActionExecuteOptions are the options passed to the execute action function
type ActionExecuteOptions struct {
	Args      []string
	Viper     *viper.Viper
	Logger    *zap.Logger
	TLSConfig *ClientConfig
}

// ActionCreatorFunction type is the function type in charge of create the action to be executed
type ActionCreatorFunction func(client.Client, Config) Action

func getTLSConfig(tlsConfig *ClientConfig, logger *zap.Logger) (*tls.Config, error) {
	if tlsConfig == nil {
		return nil, nil
	}

	if tlsConfig.Insecure {
		logger.Info("TLS is disabled")
		return nil, nil
	}

	ctx := context.Background()

	return tlsConfig.LoadTLSConfig(ctx)
}

// ExecuteAction execute the action returned by the createAction function
func ExecuteAction(opts ActionExecuteOptions, createAction ActionCreatorFunction) error {
	cfg := Config{}
	cfg.InitFromViper(opts.Viper)
	tlsCfg, err := getTLSConfig(opts.TLSConfig, opts.Logger)
	if err != nil {
		return err
	}

	esClient := newESClient(opts.Args[0], &cfg, tlsCfg)
	action := createAction(esClient, cfg)
	return action.Do()
}
