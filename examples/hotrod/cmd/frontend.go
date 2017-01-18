package cmd

import (
	"net"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/uber-go/zap"

	"github.com/uber/jaeger/examples/hotrod/pkg/log"
	"github.com/uber/jaeger/examples/hotrod/pkg/tracing"
	"github.com/uber/jaeger/examples/hotrod/services/frontend"
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
			tracing.Init("frontend", logger),
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
	frontendCmd.Flags().IntVarP(&frontendOptions.serverPort, "port", "p", 8080, "port on which the frontend server will listen")
}
