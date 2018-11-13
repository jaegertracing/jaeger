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
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	jMetrics "github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc/grpclog"

	"github.com/jaegertracing/jaeger/cmd/agent/app"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/version"
)

func main() {
	v := viper.New()
	var command = &cobra.Command{
		Use:   "jaeger-agent",
		Short: "Jaeger agent is a local daemon program which collects tracing data.",
		Long:  `Jaeger agent is a daemon program that runs on every host and receives tracing data submitted by Jaeger client libraries.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := flags.TryLoadConfigFile(v)
			if err != nil {
				return err
			}

			sFlags := new(flags.SharedFlags).InitFromViper(v)
			logger, err := sFlags.NewLogger(zap.NewProductionConfig())
			if err != nil {
				return err
			}

			builder := &app.Builder{}
			builder.InitFromViper(v)
			mBldr := new(metrics.Builder).InitFromViper(v)

			mFactory, err := mBldr.CreateMetricsFactory("jaeger")
			if err != nil {
				logger.Fatal("Could not create metrics", zap.Error(err))
			}
			mFactory = mFactory.Namespace("agent", nil)

			rOpts := new(reporter.Options).InitFromViper(v)
			tChanOpts := new(tchannel.Builder).InitFromViper(v, logger)
			grpcOpts := new(grpc.Options).InitFromViper(v)
			cp, err := createCollectorProxy(rOpts, tChanOpts, grpcOpts, logger, mFactory)
			if err != nil {
				logger.Fatal("Could not create collector proxy", zap.Error(err))
			}

			// TODO illustrate discovery service wiring

			agent, err := builder.CreateAgent(cp, logger, mFactory)
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

	command.AddCommand(version.Command())

	config.AddFlags(
		v,
		command,
		flags.AddConfigFileFlag,
		flags.AddLoggingFlag,
		app.AddFlags,
		reporter.AddFlags,
		tchannel.AddFlags,
		grpc.AddFlags,
		metrics.AddFlags,
	)

	if err := command.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func createCollectorProxy(
	opts *reporter.Options,
	tchanRep *tchannel.Builder,
	grpcRepOpts *grpc.Options,
	logger *zap.Logger,
	mFactory jMetrics.Factory,
) (app.CollectorProxy, error) {
	switch opts.ReporterType {
	case reporter.GRPC:
		grpclog.SetLoggerV2(grpclog.NewLoggerV2(ioutil.Discard, os.Stderr, os.Stderr))
		return grpc.NewCollectorProxy(grpcRepOpts, logger)
	case reporter.TCHANNEL:
		return tchannel.NewCollectorProxy(tchanRep, mFactory, logger)
	default:
		return nil, errors.New(fmt.Sprintf("unknown reporter type %s", string(opts.ReporterType)))
	}
}
