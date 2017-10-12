// Copyright (c) 2017 Uber Technologies, Inc.
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
	"runtime"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/uber/jaeger/cmd/agent/app"
	"github.com/uber/jaeger/cmd/flags"
	"github.com/uber/jaeger/pkg/config"
	"github.com/uber/jaeger/pkg/metrics"
)

func main() {
	logger, _ := zap.NewProduction()
	v := viper.New()
	var command = &cobra.Command{
		Use:   "jaeger-agent",
		Short: "Jaeger agent is a local daemon program which collects tracing data.",
		Long:  `Jaeger agent is a daemon program that runs on every host and receives tracing data submitted by Jaeger client libraries.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			flags.TryLoadConfigFile(v, logger)

			builder := &app.Builder{}
			builder.InitFromViper(v)
			runtime.GOMAXPROCS(runtime.NumCPU())

			// TODO illustrate discovery service wiring
			// TODO illustrate additional reporter

			agent, err := builder.CreateAgent(logger)
			if err != nil {
				return errors.Wrap(err, "Unable to initialize Jaeger Agent")
			}

			logger.Info("Starting agent")
			if err := agent.Run(); err != nil {
				return errors.Wrap(err, "Failed to run the agent")
			}
			select {}
		},
	}

	config.AddFlags(
		v,
		command,
		flags.AddConfigFileFlag,
		app.AddFlags,
		metrics.AddFlags,
	)

	if err := command.Execute(); err != nil {
		logger.Fatal("agent command failed", zap.Error(err))
	}
}
