// Copyright © 2017 Uber Technologies, Inc.
//

package cmd

import (
	"net"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/uber-go/zap"

	"code.uber.internal/infra/jaeger-demo/pkg/log"
	"code.uber.internal/infra/jaeger-demo/pkg/tracing"
	"code.uber.internal/infra/jaeger-demo/services/route"
)

// routeCmd represents the route command
var routeCmd = &cobra.Command{
	Use:   "route",
	Short: "Starts Route service",
	Long:  `Starts Route service.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := log.NewFactory(logger.With(zap.String("service", "route")))
		server := route.NewServer(
			net.JoinHostPort(routeOptions.serverInterface, strconv.Itoa(routeOptions.serverPort)),
			tracing.Init("route", logger),
			logger,
		)
		return server.Run()
	},
}

var (
	routeOptions struct {
		serverInterface string
		serverPort      int
	}
)

func init() {
	RootCmd.AddCommand(routeCmd)

	routeCmd.Flags().StringVarP(&routeOptions.serverInterface, "bind", "", "127.0.0.1", "interface to which the Route server will bind")
	routeCmd.Flags().IntVarP(&routeOptions.serverPort, "port", "p", 8083, "port on which the Route server will listen")
}
