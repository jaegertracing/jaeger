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
		logger := log.NewFactory(logger.With(zap.String("service", "frontend")))
		server := frontend.NewServer(
			net.JoinHostPort(frontendOptions.serverInterface, strconv.Itoa(frontendOptions.serverPort)),
			tracing.Init("frontend", metricsFactory.Namespace("frontend", nil), logger),
			logger,
		)
		return server.Run()
	},
}

var (
	frontendOptions struct {
		serverInterface string
		serverPort      int
	}
)

func init() {
	RootCmd.AddCommand(frontendCmd)

	frontendCmd.Flags().StringVarP(&frontendOptions.serverInterface, "bind", "", "127.0.0.1", "interface to which the frontend server will bind")
	frontendCmd.Flags().IntVarP(&frontendOptions.serverPort, "port", "p", 8099, "port on which the frontend server will listen")
}
