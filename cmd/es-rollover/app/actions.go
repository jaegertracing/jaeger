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

package app

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
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

// ActionExecuteOptions are the options passed to the execute action function
type ActionExecuteOptions struct {
	Args     []string
	Viper    *viper.Viper
	Logger   *zap.Logger
	TLSFlags tlscfg.ClientFlagsConfig
}

// ActionCreatorFunction type is the function type in charge of create the action to be executed
type ActionCreatorFunction func(client.Client, Config) Action

// ExecuteAction execute the action returned by the createAction function
func ExecuteAction(opts ActionExecuteOptions, createAction ActionCreatorFunction) error {
	cfg := Config{}
	cfg.InitFromViper(opts.Viper)
	tlsOpts, err := opts.TLSFlags.InitFromViper(opts.Viper)
	if err != nil {
		return err
	}
	tlsCfg, err := tlsOpts.Config(opts.Logger)
	if err != nil {
		return err
	}
	defer tlsOpts.Close()

	esClient := newESClient(opts.Args[0], &cfg, tlsCfg)
	action := createAction(esClient, cfg)
	return action.Do()
}
