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
	"github.com/jaegertracing/jaeger/examples/hotrod/services/route"
)

// routeCmd represents the route command
var routeCmd = &cobra.Command{
	Use:   "route",
	Short: "Starts Route service",
	Long:  `Starts Route service.`,
	RunE: func(_ *cobra.Command, _ /* args */ []string) error {
		zapLogger := logger.With(zap.String("service", "route"))
		logger := log.NewFactory(zapLogger)
		server := route.NewServer(
			net.JoinHostPort("0.0.0.0", strconv.Itoa(routePort)),
			tracing.InitOTEL("route", otelExporter, metricsFactory, logger),
			logger,
		)
		return logError(zapLogger, server.Run())
	},
}

func init() {
	RootCmd.AddCommand(routeCmd)
}
