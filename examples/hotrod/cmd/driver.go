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
	"github.com/jaegertracing/jaeger/examples/hotrod/services/driver"
)

// driverCmd represents the driver command
var driverCmd = &cobra.Command{
	Use:   "driver",
	Short: "Starts Driver service",
	Long:  `Starts Driver service.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := log.NewFactory(logger.With(zap.String("service", "driver")))
		server := driver.NewServer(
			net.JoinHostPort(driverOptions.serverInterface, strconv.Itoa(driverOptions.serverPort)),
			tracing.Init("driver", metricsFactory.Namespace("driver", nil), logger, jAgentHostPort),
			metricsFactory,
			logger,
			jAgentHostPort,
		)
		return server.Run()
	},
}

var (
	driverOptions struct {
		serverInterface string
		serverPort      int
	}
)

func init() {
	RootCmd.AddCommand(driverCmd)

	driverCmd.Flags().StringVarP(&driverOptions.serverInterface, "bind", "", "127.0.0.1", "interface to which the driver server will bind")
	driverCmd.Flags().IntVarP(&driverOptions.serverPort, "port", "p", 8082, "port on which the driver server will listen")
}
