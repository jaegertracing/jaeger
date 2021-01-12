// Copyright (c) 2019 The Jaeger Authors.
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
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	jexpvar "github.com/uber/jaeger-lib/metrics/expvar"
	"github.com/uber/jaeger-lib/metrics/fork"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	"github.com/jaegertracing/jaeger/cmd/docs"
	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/status"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/version"
	"github.com/jaegertracing/jaeger/ports"
)

func main() {
	svc := flags.NewService(ports.AgentAdminHTTP)
	svc.NoStorage = true

	v := viper.New()
	var command = &cobra.Command{
		Use:   "jaeger-agent",
		Short: "Jaeger agent is a local daemon program which collects tracing data.",
		Long:  `Jaeger agent is a daemon program that runs on every host and receives tracing data submitted by Jaeger client libraries.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.Start(v); err != nil {
				return err
			}
			logger := svc.Logger // shortcut
			baseFactory := svc.MetricsFactory.
				Namespace(metrics.NSOptions{Name: "jaeger"}).
				Namespace(metrics.NSOptions{Name: "agent"})
			mFactory := fork.New("internal",
				jexpvar.NewFactory(10), // backend for internal opts
				baseFactory)

			rOpts := new(reporter.Options).InitFromViper(v, logger)
			grpcBuilder := grpc.NewConnBuilder().InitFromViper(v)
			builders := map[reporter.Type]app.CollectorProxyBuilder{
				reporter.GRPC: app.GRPCCollectorProxyBuilder(grpcBuilder),
			}
			cp, err := app.CreateCollectorProxy(app.ProxyBuilderOptions{
				Options: *rOpts,
				Logger:  logger,
				Metrics: mFactory,
			}, builders)
			if err != nil {
				logger.Fatal("Could not create collector proxy", zap.Error(err))
			}

			// TODO illustrate discovery service wiring

			builder := new(app.Builder).InitFromViper(v)
			agent, err := builder.CreateAgent(cp, logger, mFactory)
			if err != nil {
				return fmt.Errorf("unable to initialize Jaeger Agent: %w", err)
			}

			logger.Info("Starting agent")
			if err := agent.Run(); err != nil {
				return fmt.Errorf("failed to run the agent: %w", err)
			}

			svc.RunAndThen(func() {
				agent.Stop()
				cp.Close()
			})
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(status.Command(v, ports.AgentAdminHTTP))

	config.AddFlags(
		v,
		command,
		svc.AddFlags,
		app.AddFlags,
		reporter.AddFlags,
		grpc.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
