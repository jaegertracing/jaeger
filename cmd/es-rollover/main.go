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
	"flag"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/es-rollover/app"
	initialize "github.com/jaegertracing/jaeger/cmd/es-rollover/app/init"
	"github.com/jaegertracing/jaeger/cmd/es-rollover/app/lookback"
	"github.com/jaegertracing/jaeger/cmd/es-rollover/app/rollover"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/es/client"
)

func main() {
	v := viper.New()
	logger, _ := zap.NewProduction()

	rootCmd := &cobra.Command{
		Use:   "jaeger-es-rollover",
		Short: "Jaeger es-rollover manages Jaeger indices",
		Long:  "Jaeger es-rollover manages Jaeger indices",
	}

	tlsFlags := tlscfg.ClientFlagsConfig{Prefix: "es"}

	// Init command
	initCfg := &initialize.Config{}
	initCommand := &cobra.Command{
		Use:          "init http://HOSTNAME:PORT",
		Short:        "creates indices and aliases",
		Long:         "creates indices and aliases",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ExecuteAction(app.ActionExecuteOptions{
				Args:     args,
				Viper:    v,
				Logger:   logger,
				TLSFlags: tlsFlags,
			}, func(c client.Client, cfg app.Config) app.Action {
				initCfg.Config = cfg
				initCfg.InitFromViper(v)
				indicesClient := &client.IndicesClient{
					Client:               c,
					MasterTimeoutSeconds: initCfg.Timeout,
				}
				clusterClient := &client.ClusterClient{
					Client: c,
				}
				ilmClient := &client.ILMClient{
					Client: c,
				}
				return &initialize.Action{
					IndicesClient: indicesClient,
					ClusterClient: clusterClient,
					ILMClient:     ilmClient,
					Config:        *initCfg,
				}
			})
		},
	}

	// Rollover command
	rolloverCfg := &rollover.Config{}

	rolloverCommand := &cobra.Command{
		Use:   "rollover http://HOSTNAME:PORT",
		Short: "rollover to new write index",
		Long:  "rollover to new write index",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rolloverCfg.InitFromViper(v)
			return app.ExecuteAction(app.ActionExecuteOptions{
				Args:     args,
				Viper:    v,
				Logger:   logger,
				TLSFlags: tlsFlags,
			}, func(c client.Client, cfg app.Config) app.Action {
				rolloverCfg.Config = cfg
				rolloverCfg.InitFromViper(v)
				indicesClient := &client.IndicesClient{
					Client:               c,
					MasterTimeoutSeconds: rolloverCfg.Timeout,
				}

				return &rollover.Action{
					IndicesClient: indicesClient,
					Config:        *rolloverCfg,
				}
			})
		},
	}

	lookbackCfg := lookback.Config{}
	lookbackCommand := &cobra.Command{
		Use:   "lookback http://HOSTNAME:PORT",
		Short: "removes old indices from read alias",
		Long:  "removes old indices from read alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lookbackCfg.InitFromViper(v)
			return app.ExecuteAction(app.ActionExecuteOptions{
				Args:     args,
				Viper:    v,
				Logger:   logger,
				TLSFlags: tlsFlags,
			}, func(c client.Client, cfg app.Config) app.Action {
				lookbackCfg.Config = cfg
				lookbackCfg.InitFromViper(v)
				indicesClient := &client.IndicesClient{
					Client:               c,
					MasterTimeoutSeconds: lookbackCfg.Timeout,
				}
				return &lookback.Action{
					IndicesClient: indicesClient,
					Config:        lookbackCfg,
					Logger:        logger,
				}
			})
		},
	}

	addPersistentFlags(v, rootCmd, tlsFlags.AddFlags, app.AddFlags)
	addSubCommand(v, rootCmd, initCommand, initCfg.AddFlags)
	addSubCommand(v, rootCmd, rolloverCommand, rolloverCfg.AddFlags)
	addSubCommand(v, rootCmd, lookbackCommand, lookbackCfg.AddFlags)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func addSubCommand(v *viper.Viper, rootCmd, cmd *cobra.Command, addFlags func(*flag.FlagSet)) {
	rootCmd.AddCommand(cmd)
	config.AddFlags(
		v,
		cmd,
		addFlags,
	)
}

func addPersistentFlags(v *viper.Viper, rootCmd *cobra.Command, inits ...func(*flag.FlagSet)) {
	flagSet := new(flag.FlagSet)
	for i := range inits {
		inits[i](flagSet)
	}
	rootCmd.PersistentFlags().AddGoFlagSet(flagSet)
	v.BindPFlags(rootCmd.PersistentFlags())
}
