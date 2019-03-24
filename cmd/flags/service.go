package flags

import (
	"flag"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc/grpclog"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	pMetrics "github.com/jaegertracing/jaeger/pkg/metrics"
)

// Service represents an abstract Jaeger backend component with some basic shared functionality.
type Service struct {
	// AdminPort is the HTTP port number for admin server.
	AdminPort int

	// NoStorage indicates that storage type CLI flag is not applicable
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
	signalsChannel := make(chan os.Signal)
	signal.Notify(signalsChannel, os.Interrupt, syscall.SIGTERM)

	grpclog.SetLoggerV2(grpclog.NewLoggerV2(ioutil.Discard, os.Stderr, os.Stderr))

	return &Service{
		Admin:          NewAdminServer(adminPort),
		signalsChannel: signalsChannel,
	}
}

// AddFlags registers CLI flags.
func (s *Service) AddFlags(flagSet *flag.FlagSet) {
	AddConfigFileFlag(flagSet)
	if s.NoStorage {
		AddLoggingFlag(flagSet)
	} else {
		AddFlags(flagSet)
	}
	pMetrics.AddFlags(flagSet)
	s.Admin.AddFlags(flagSet)
}

// Start bootstraps the service and starts the admin server.
func (s *Service) Start(v *viper.Viper) error {
	if err := TryLoadConfigFile(v); err != nil {
		return errors.Wrap(err, "Cannot load config file")
	}

	sFlags := new(SharedFlags).InitFromViper(v)
	if logger, err := sFlags.NewLogger(zap.NewProductionConfig()); err == nil {
		s.Logger = logger
	} else {
		return errors.Wrap(err, "Cannot create logger")
	}

	metricsBuilder := new(pMetrics.Builder).InitFromViper(v)
	metricsFactory, err := metricsBuilder.CreateMetricsFactory("")
	if err != nil {
		return errors.Wrap(err, "Cannot create metrics factory")
	}
	s.MetricsFactory = metricsFactory

	s.Admin.initFromViper(v, s.Logger)
	if h := metricsBuilder.Handler(); h != nil {
		route := metricsBuilder.HTTPRoute
		s.Logger.Info("Registering metrics handler with admin server", zap.String("route", route))
		s.Admin.Handle(route, h)
	}
	if err := s.Admin.Serve(); err != nil {
		return errors.Wrap(err, "Cannot start the admin server")
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

	if shutdown != nil {
		shutdown()
	}

	s.Logger.Info("Shutdown complete")
}
