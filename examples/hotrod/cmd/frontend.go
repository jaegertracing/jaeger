// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

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
	RunE: func(_ *cobra.Command, _ /* args */ []string) error {
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
			tracing.InitOTEL("frontend", otelExporter, metricsFactory, logger),
			logger,
		)
		return logError(zapLogger, server.Run())
	},
}

var options frontend.ConfigOptions

func init() {
	RootCmd.AddCommand(frontendCmd)
}
