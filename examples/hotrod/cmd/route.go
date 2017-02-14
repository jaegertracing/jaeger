// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cmd

import (
	"net"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/uber-go/zap"

	"github.com/uber/jaeger/examples/hotrod/pkg/log"
	"github.com/uber/jaeger/examples/hotrod/pkg/tracing"
	"github.com/uber/jaeger/examples/hotrod/services/route"
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
			tracing.Init("route", metricsFactory.Namespace("route", nil), logger),
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
