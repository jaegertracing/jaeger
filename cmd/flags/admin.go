package flags

import (
	"context"
	"flag"
	"net"
	"net/http"
	"strconv"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
)

const (
	adminHTTPPort       = "admin-http-port"
	healthCheckHTTPPort = "health-check-http-port"
)

// AdminServer runs an HTTP server with admin endpoints, such as healthcheck at /, /metrics, etc.
type AdminServer struct {
	logger    *zap.Logger
	adminPort int

	hc *healthcheck.HealthCheck

	mux    *http.ServeMux
	server *http.Server
}

// NewAdminServer creates a new admin server.
func NewAdminServer(defaultPort int) *AdminServer {
	return &AdminServer{
		adminPort: defaultPort,
		logger:    zap.NewNop(),
		hc:        healthcheck.New(),
		mux:       http.NewServeMux(),
	}
}

// HC returns the reference to HeathCheck.
func (s *AdminServer) HC() *healthcheck.HealthCheck {
	return s.hc
}

// SetLogger initializes logger.
func (s *AdminServer) SetLogger(logger *zap.Logger) {
	s.logger = logger
	s.hc.SetLogger(logger)
}

// AddFlags registers CLI flags.
func (s *AdminServer) AddFlags(flagSet *flag.FlagSet) {
	flagSet.Int(healthCheckHTTPPort, 0, "(deprecated) The http port for the health check service")
	flagSet.Int(adminHTTPPort, s.adminPort, "The http port for the admin server, including health check, /metrics, etc.")
}

// InitFromViper initializes the server with properties retrieved from Viper.
func (s *AdminServer) initFromViper(v *viper.Viper, logger *zap.Logger) {
	s.SetLogger(logger)
	s.adminPort = v.GetInt(adminHTTPPort)
	if v := v.GetInt(healthCheckHTTPPort); v != 0 {
		logger.Sugar().Warnf("Using deprecated flag %s, please upgrade to %s", healthCheckHTTPPort, adminHTTPPort)
		s.adminPort = v
	}
}

// Handle adds a new handler to the admin server.
func (s *AdminServer) Handle(path string, handler http.Handler) {
	s.mux.Handle(path, handler)
}

// Serve starts HTTP server.
func (s *AdminServer) Serve() error {
	s.logger.Info("Starting admin HTTP server", zap.Int("http-port", s.adminPort))
	portStr := ":" + strconv.Itoa(s.adminPort)
	l, err := net.Listen("tcp", portStr)
	if err != nil {
		s.logger.Error("Admin server failed to listen", zap.Error(err))
		return err
	}
	s.serveWithListener(l)
	s.logger.Info(
		"Admin server started",
		zap.Int("http-port", s.adminPort),
		zap.Stringer("health-status", s.hc.Get()))
	return nil
}

func (s *AdminServer) serveWithListener(l net.Listener) {
	s.mux.Handle("/", s.hc.Handler())
	recoveryHandler := recoveryhandler.NewRecoveryHandler(s.logger, true)
	s.server = &http.Server{Handler: recoveryHandler(s.mux)}
	go func() {
		if err := s.server.Serve(l); err != nil {
			s.logger.Error("failed to serve", zap.Error(err))
			s.hc.Set(healthcheck.Broken)
		}
	}()
}

// Close stops the HTTP server
func (s *AdminServer) Close() error {
	return s.server.Shutdown(context.Background())
}
