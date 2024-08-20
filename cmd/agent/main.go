// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	"github.com/jaegertracing/jaeger/cmd/internal/docs"
	"github.com/jaegertracing/jaeger/cmd/internal/flags"
	"github.com/jaegertracing/jaeger/cmd/internal/status"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/version"
	"github.com/jaegertracing/jaeger/ports"
)

func main() {
	println("***************************************************************************************************")
	println("*** WARNING jaeger-agent is deprecated. See https://github.com/jaegertracing/jaeger/issues/4739 ***")
	println("***************************************************************************************************")

	svc := flags.NewService(ports.AgentAdminHTTP)
	svc.NoStorage = true

	v := viper.New()
	command := &cobra.Command{
		Use:   "jaeger-agent",
		Short: "(deprecated) Jaeger agent is a local daemon program which collects tracing data.",
		Long:  `(deprecated) Jaeger agent is a daemon program that runs on every host and receives tracing data submitted by Jaeger client libraries.`,
		RunE: func(_ *cobra.Command, _ /* args */ []string) error {
			if err := svc.Start(v); err != nil {
				return err
			}
			logger := svc.Logger // shortcut

			mFactory := svc.MetricsFactory.
				Namespace(metrics.NSOptions{Name: "jaeger"}).
				Namespace(metrics.NSOptions{Name: "agent"})
			version.NewInfoMetrics(mFactory)

			rOpts := new(reporter.Options).InitFromViper(v, logger)
			grpcBuilder, err := grpc.NewConnBuilder().InitFromViper(v)
			if err != nil {
				logger.Fatal("Failed to configure gRPC connection", zap.Error(err))
			}
			builders := map[reporter.Type]app.CollectorProxyBuilder{
				reporter.GRPC: app.GRPCCollectorProxyBuilder(grpcBuilder),
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cp, err := app.CreateCollectorProxy(ctx, app.ProxyBuilderOptions{
				Options: *rOpts,
				Logger:  logger,
				Metrics: mFactory,
			}, builders)
			if err != nil {
				logger.Fatal("Failed to create collector proxy", zap.Error(err))
			}

			// TODO illustrate discovery service wiring

			builder := new(app.Builder).InitFromViper(v)
			agent, err := builder.CreateAgent(cp, logger, mFactory)
			if err != nil {
				return fmt.Errorf("failed to initialize Jaeger Agent: %w", err)
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
