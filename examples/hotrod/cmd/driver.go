// Copyright © 2017 Uber Technologies, Inc.
//

package cmd

import (
	"net"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/uber-go/zap"

	"github.com/uber/jaeger/examples/hotrod/pkg/log"
	"github.com/uber/jaeger/examples/hotrod/pkg/tracing"
	"github.com/uber/jaeger/examples/hotrod/services/driver"
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
			tracing.Init("driver", logger),
			logger,
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
