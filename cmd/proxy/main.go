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

	"github.com/opentracing/opentracing-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	jaegerClientZapLog "github.com/uber/jaeger-client-go/log/zap"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/docs"
	"github.com/jaegertracing/jaeger/cmd/env"
	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/cmd/proxy/app"
	"github.com/jaegertracing/jaeger/cmd/proxy/app/proxysvc"
	"github.com/jaegertracing/jaeger/cmd/status"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/version"
	"github.com/jaegertracing/jaeger/ports"
)

func main() {
	svc := flags.NewService(ports.ProxyAdminHTTP)

	v := viper.New()
	var command = &cobra.Command{
		Use:   "jaeger-proxy",
		Short: "Jaeger proxy service provides a Web UI and an API for accessing trace data.",
		Long:  `Jaeger proxy service provides a Web UI and an API for accessing trace data.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := svc.Start(v); err != nil {
				return err
			}
			logger := svc.Logger // shortcut
			// baseFactory := svc.MetricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"})
			traceCfg := &jaegerClientConfig.Configuration{
				ServiceName: "jaeger-proxy",
				Sampler: &jaegerClientConfig.SamplerConfig{
					Type:  "const",
					Param: 1.0,
				},
				RPCMetrics: true,
			}
			traceCfg, err := traceCfg.FromEnv()
			if err != nil {
				logger.Fatal("Failed to read tracer configuration", zap.Error(err))
			}
			tracer, closer, err := traceCfg.NewTracer(
				jaegerClientConfig.Metrics(svc.MetricsFactory),
				jaegerClientConfig.Logger(jaegerClientZapLog.NewLogger(logger)),
			)
			if err != nil {
				logger.Fatal("Failed to initialize tracer", zap.Error(err))
			}
			defer closer.Close()
			opentracing.SetGlobalTracer(tracer)
			proxyOpts := new(app.ProxyOptions).InitFromViper(v, logger)
			proxyServiceOptions := proxyOpts.BuildProxyServiceOptions(logger)
			proxyService := proxysvc.NewProxyService(*proxyServiceOptions, logger)

			server, err := app.NewServer(svc.Logger, proxyService, proxyOpts, tracer)
			if err != nil {
				logger.Fatal("Failed to create server", zap.Error(err))
			}

			go func() {
				for s := range server.HealthCheckStatus() {
					svc.SetHealthCheckStatus(s)
				}
			}()

			if err := server.Start(); err != nil {
				logger.Fatal("Could not start servers", zap.Error(err))
			}

			svc.RunAndThen(func() {
				server.Close()
			})
			return nil
		},
	}

	command.AddCommand(version.Command())
	command.AddCommand(env.Command())
	command.AddCommand(docs.Command(v))
	command.AddCommand(status.Command(v, ports.ProxyAdminHTTP))

	config.AddFlags(
		v,
		command,
		svc.AddFlags,
		app.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
