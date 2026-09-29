// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"expvar"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/featuregate"
	"go.uber.org/zap"
	"go.uber.org/zap/zapgrpc"
	"google.golang.org/grpc/grpclog"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metrics/metricsbuilder"
	"github.com/jaegertracing/jaeger/ports"
)

// ServiceConfig holds the settings every service built on Service reads from its
// configuration file: the admin server, logging and the metrics backend.
type ServiceConfig struct {
	Admin   confighttp.ServerConfig `mapstructure:"admin"`
	Logging LoggingConfig           `mapstructure:"logging"`
	Metrics metricsbuilder.Builder  `mapstructure:"metrics"`
}

// DefaultServiceConfig returns the settings a service runs with when its configuration
// file does not name them: an admin server on adminPort, info-level JSON logs, and
// Prometheus metrics on /metrics.
func DefaultServiceConfig(adminPort int) ServiceConfig {
	return ServiceConfig{
		Admin: confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ports.PortToHostPort(adminPort),
				Transport: confignet.TransportTypeTCP,
			},
		},
		Logging: LoggingConfig{
			Level:    "info",
			Encoding: "json",
		},
		Metrics: metricsbuilder.Default(),
	}
}

// Service represents an abstract Jaeger backend component with some basic shared functionality.
type Service struct {
	// AdminPort is the HTTP port number for admin server.
	AdminPort int

	// Admin is the admin server that hosts the health check and metrics endpoints.
	Admin *AdminServer

	// Logger is initialized by Start from the logging configuration.
	Logger *zap.Logger

	// MetricsFactory is the root factory without a namespace.
	MetricsFactory metrics.Factory

	signalsChannel chan os.Signal
}

// NewService creates a new Service.
func NewService(adminPort int) *Service {
	signalsChannel := make(chan os.Signal, 1)
	signal.Notify(signalsChannel, os.Interrupt, syscall.SIGTERM)

	return &Service{
		Admin:          NewAdminServer(ports.PortToHostPort(adminPort)),
		signalsChannel: signalsChannel,
	}
}

// AddFlags registers the CLI flags: the configuration file, which carries every other
// setting, and the feature gates.
func (*Service) AddFlags(flagSet *flag.FlagSet) {
	AddConfigFileFlag(flagSet)
	featuregate.GlobalRegistry().RegisterFlags(flagSet)
}

// Start bootstraps the service from its configuration and starts the admin server.
func (s *Service) Start(cfg ServiceConfig) error {
	newProdConfig := zap.NewProductionConfig()
	newProdConfig.Sampling = nil
	logger, err := cfg.Logging.NewLogger(newProdConfig)
	if err != nil {
		return fmt.Errorf("cannot create logger: %w", err)
	}
	s.Logger = logger
	grpclog.SetLoggerV2(zapgrpc.NewLogger(
		logger.WithOptions(
			zap.AddCallerSkip(5), // ensure the actual caller:lineNo is shown
		),
	))

	metricsBuilder := cfg.Metrics
	metricsFactory, err := metricsBuilder.CreateMetricsFactory("")
	if err != nil {
		return fmt.Errorf("cannot create metrics factory: %w", err)
	}
	s.MetricsFactory = metricsFactory

	s.Admin.configure(cfg.Admin, s.Logger)
	if h := metricsBuilder.Handler(); h != nil {
		route := metricsBuilder.HTTPRoute
		s.Logger.Info("Mounting metrics handler on admin server", zap.String("route", route))
		s.Admin.Handle(route, h)
	}

	// Mount expvar routes on different backends
	if metricsBuilder.Backend != "expvar" {
		s.Logger.Info("Mounting expvar handler on admin server", zap.String("route", "/debug/vars"))
		s.Admin.Handle("/debug/vars", expvar.Handler())
	}

	if err := s.Admin.Serve(); err != nil {
		return fmt.Errorf("cannot start the admin server: %w", err)
	}

	return nil
}

// RunAndThen sets the health check to Ready and blocks until SIGTERM is received.
// It then runs the shutdown function and exits.
func (s *Service) RunAndThen(shutdown func()) error {
	s.Admin.Host().Ready()

	<-s.signalsChannel

	s.Logger.Info("Shutting down")
	s.Admin.Host().SetUnavailable()

	if shutdown != nil {
		shutdown()
	}

	err := s.Admin.Close()
	if err == nil {
		s.Logger.Info("Shutdown complete")
	}
	return err
}
