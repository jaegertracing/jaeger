// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esrollover

import (
	"flag"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/es/client"
	app "github.com/jaegertracing/jaeger/pkg/esrollover"
	initialize "github.com/jaegertracing/jaeger/pkg/esrollover/init"
	"github.com/jaegertracing/jaeger/pkg/esrollover/lookback"
	"github.com/jaegertracing/jaeger/pkg/esrollover/rollover"
)

func Command(v *viper.Viper, logger *zap.Logger) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "es-rollover",
		Short: "es-rollover manages Jaeger indices",
		Long:  "es-rollover manages Jaeger indices",
	}
	addPersistentFlags(v, rootCmd, app.AddFlags)
	rootCmd.AddCommand(initCommand(v, logger))
	rootCmd.AddCommand(rolloverCommand(v, logger))
	rootCmd.AddCommand(lookBackCommand(v, logger))
	return rootCmd
}

func initCommand(v *viper.Viper, logger *zap.Logger) *cobra.Command {
	initCfg := &initialize.Config{}
	initCmd := &cobra.Command{
		Use:          "init http://HOSTNAME:PORT",
		Short:        "creates indices and aliases",
		Long:         "creates indices and aliases",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			return app.ExecuteAction(app.ActionExecuteOptions{
				Args:   args,
				Viper:  v,
				Logger: logger,
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
	addPersistentFlags(v, initCmd, initCfg.AddFlags)
	return initCmd
}

func rolloverCommand(v *viper.Viper, logger *zap.Logger) *cobra.Command {
	rolloverCfg := &rollover.Config{}
	rolloverCmd := &cobra.Command{
		Use:   "rollover http://HOSTNAME:PORT",
		Short: "rollover to new write index",
		Long:  "rollover to new write index",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rolloverCfg.InitFromViper(v)
			return app.ExecuteAction(app.ActionExecuteOptions{
				Args:   args,
				Viper:  v,
				Logger: logger,
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
	addPersistentFlags(v, rolloverCmd, rolloverCfg.AddFlags)
	return rolloverCmd
}

func lookBackCommand(v *viper.Viper, logger *zap.Logger) *cobra.Command {
	lookbackCfg := lookback.Config{}
	lookBackCmd := &cobra.Command{
		Use:   "lookback http://HOSTNAME:PORT",
		Short: "removes old indices from read alias",
		Long:  "removes old indices from read alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			lookbackCfg.InitFromViper(v)
			return app.ExecuteAction(app.ActionExecuteOptions{
				Args:   args,
				Viper:  v,
				Logger: logger,
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
	addPersistentFlags(v, lookBackCmd, lookbackCfg.AddFlags)
	return lookBackCmd
}

func addPersistentFlags(v *viper.Viper, rootCmd *cobra.Command, inits ...func(*flag.FlagSet)) {
	flagSet := new(flag.FlagSet)
	for i := range inits {
		inits[i](flagSet)
	}
	rootCmd.PersistentFlags().AddGoFlagSet(flagSet)
	v.BindPFlags(rootCmd.PersistentFlags())
}
