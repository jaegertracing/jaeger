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
	"github.com/jaegertracing/jaeger/examples/hotrod/services/driver"
)

// driverCmd represents the driver command
var driverCmd = &cobra.Command{
	Use:   "driver",
	Short: "Starts Driver service",
	Long:  `Starts Driver service.`,
	RunE: func(_ *cobra.Command, _ /* args */ []string) error {
		zapLogger := logger.With(zap.String("service", "driver"))
		logger := log.NewFactory(zapLogger)
		server := driver.NewServer(
			net.JoinHostPort("0.0.0.0", strconv.Itoa(driverPort)),
			otelExporter,
			metricsFactory,
			logger,
		)
		return logError(zapLogger, server.Run())
	},
}

func init() {
	RootCmd.AddCommand(driverCmd)
}
