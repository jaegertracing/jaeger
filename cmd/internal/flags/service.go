// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"expvar"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapgrpc"
	"google.golang.org/grpc/grpclog"

	"github.com/jaegertracing/jaeger/internal/metrics/metricsbuilder"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/ports"
)

// Service represents an abstract Jaeger backend component with some basic shared functionality.
type Service struct {
	// AdminPort is the HTTP port number for admin server.
	AdminPort int

	// NoStorage indicates that storage-type CLI flag is not applicable
	NoStorage bool

	// Admin is the admin server that hosts the health check and metrics endpoints.
	Admin *AdminServer

	// Logger is initialized after parsing Viper flags like --log-level.
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

// AddFlags registers CLI flags.
func (s *Service) AddFlags(flagSet *flag.FlagSet) {
	AddConfigFileFlag(flagSet)
	if s.NoStorage {
		AddLoggingFlags(flagSet)
	} else {
		AddFlags(flagSet)
	}
	metricsbuilder.AddFlags(flagSet)
	s.Admin.AddFlags(flagSet)
}

// Start bootstraps the service and starts the admin server.
func (s *Service) Start(v *viper.Viper) error {
	if err := TryLoadConfigFile(v); err != nil {
		return fmt.Errorf("cannot load config file: %w", err)
	}

	sFlags := new(SharedFlags).InitFromViper(v)
	newProdConfig := zap.NewProductionConfig()
	newProdConfig.Sampling = nil
	logger, err := sFlags.NewLogger(newProdConfig)
	if err != nil {
		return fmt.Errorf("cannot create logger: %w", err)
	}
	s.Logger = logger
	grpclog.SetLoggerV2(zapgrpc.NewLogger(
		logger.WithOptions(
			zap.AddCallerSkip(5), // ensure the actual caller:lineNo is shown
		)))

	metricsBuilder := new(metricsbuilder.Builder).InitFromViper(v)
	metricsFactory, err := metricsBuilder.CreateMetricsFactory("")
	if err != nil {
		return fmt.Errorf("cannot create metrics factory: %w", err)
	}
	s.MetricsFactory = metricsFactory

	if err = s.Admin.initFromViper(v, s.Logger); err != nil {
		return fmt.Errorf("cannot initialize admin server: %w", err)
	}
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

// HC returns the reference to HeathCheck.
func (s *Service) HC() *healthcheck.HealthCheck {
	return s.Admin.HC()
}

// RunAndThen sets the health check to Ready and blocks until SIGTERM is received.
// If then runs the shutdown function and exits.
func (s *Service) RunAndThen(shutdown func()) {
	s.HC().Ready()

	<-s.signalsChannel

	s.Logger.Info("Shutting down")
	s.HC().Set(healthcheck.Unavailable)

	if shutdown != nil {
		shutdown()
	}

	s.Admin.Close()
	s.Logger.Info("Shutdown complete")
}
