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
			tracing.Init("driver", metricsFactory.Namespace("driver", nil), logger),
			metricsFactory,
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
