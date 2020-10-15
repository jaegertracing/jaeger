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

package cmd

import (
	"net"
	"strconv"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing"
	"github.com/jaegertracing/jaeger/examples/hotrod/services/frontend"
)

// frontendCmd represents the frontend command
var frontendCmd = &cobra.Command{
	Use:   "frontend",
	Short: "Starts Frontend service",
	Long:  `Starts Frontend service.`,
	RunE: func(cmd *cobra.Command, args []string) error {

		options.FrontendHostPort = net.JoinHostPort("0.0.0.0", strconv.Itoa(frontendPort))
		options.DriverHostPort = net.JoinHostPort("0.0.0.0", strconv.Itoa(driverPort))
		options.CustomerHostPort = net.JoinHostPort("0.0.0.0", strconv.Itoa(customerPort))
		options.RouteHostPort = net.JoinHostPort("0.0.0.0", strconv.Itoa(routePort))
		options.Basepath = basepath
		options.JaegerUI = jaegerUI

		zapLogger := logger.With(zap.String("service", "frontend"))
		logger := log.NewFactory(zapLogger)
		server := frontend.NewServer(
			options,
			tracing.Init("frontend", metricsFactory, logger),
			logger,
		)
		return logError(zapLogger, server.Run())
	},
}

var options frontend.ConfigOptions

func init() {
	RootCmd.AddCommand(frontendCmd)

}
