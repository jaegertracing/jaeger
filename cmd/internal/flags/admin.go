// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"sync"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/pkg/version"
)

const (
	adminHTTPHostPort = "admin.http.host-port"
)

var tlsAdminHTTPFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "admin.http",
}

// AdminServer runs an HTTP server with admin endpoints, such as healthcheck at /, /metrics, etc.
type AdminServer struct {
	logger    *zap.Logger
	hc        *healthcheck.HealthCheck
	mux       *http.ServeMux
	server    *http.Server
	serverCfg confighttp.ServerConfig
	stopped   sync.WaitGroup
}

// NewAdminServer creates a new admin server.
func NewAdminServer(hostPort string) *AdminServer {
	return &AdminServer{
		logger: zap.NewNop(),
		hc:     healthcheck.New(),
		mux:    http.NewServeMux(),
		serverCfg: confighttp.ServerConfig{
			Endpoint: hostPort,
		},
	}
}

// HC returns the reference to HeathCheck.
func (s *AdminServer) HC() *healthcheck.HealthCheck {
	return s.hc
}

// setLogger initializes logger.
func (s *AdminServer) setLogger(logger *zap.Logger) {
	s.logger = logger
	s.hc.SetLogger(logger)
}

// AddFlags registers CLI flags.
func (s *AdminServer) AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(adminHTTPHostPort, s.serverCfg.Endpoint, fmt.Sprintf("The host:port (e.g. 127.0.0.1%s or %s) for the admin server, including health check, /metrics, etc.", s.serverCfg.Endpoint, s.serverCfg.Endpoint))
	tlsAdminHTTPFlagsConfig.AddFlags(flagSet)
}

// InitFromViper initializes the server with properties retrieved from Viper.
func (s *AdminServer) initFromViper(v *viper.Viper, logger *zap.Logger) error {
	s.setLogger(logger)

	tlsAdminHTTP, err := tlsAdminHTTPFlagsConfig.InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse admin server TLS options: %w", err)
	}

	s.serverCfg.Endpoint = v.GetString(adminHTTPHostPort)
	s.serverCfg.TLSSetting = tlsAdminHTTP.ToOtelServerConfig()
	return nil
}

// Handle adds a new handler to the admin server.
func (s *AdminServer) Handle(path string, handler http.Handler) {
	s.mux.Handle(path, handler)
}

// Serve starts HTTP server.
func (s *AdminServer) Serve() error {
	l, err := s.serverCfg.ToListener(context.Background())
	if err != nil {
		s.logger.Error("Admin server failed to listen", zap.Error(err))
		return err
	}

	return s.serveWithListener(l)
}

func (s *AdminServer) serveWithListener(l net.Listener) (err error) {
	s.logger.Info("Mounting health check on admin server", zap.String("route", "/"))
	s.mux.Handle("/", s.hc.Handler())
	version.RegisterHandler(s.mux, s.logger)
	s.registerPprofHandlers()
	recoveryHandler := recoveryhandler.NewRecoveryHandler(s.logger, true)
	s.server, err = s.serverCfg.ToServer(
		context.Background(),
		nil, // host
		telemetry.NoopSettings().ToOtelComponent(),
		recoveryHandler(s.mux),
	)
	if err != nil {
		return fmt.Errorf("failed to create admin server: %w", err)
	}
	errorLog, _ := zap.NewStdLogAt(s.logger, zapcore.ErrorLevel)
	s.server.ErrorLog = errorLog

	s.logger.Info("Starting admin HTTP server")
	var wg sync.WaitGroup
	wg.Add(1)
	s.stopped.Add(1)
	go func() {
		wg.Done()
		defer s.stopped.Done()
		err := s.server.Serve(l)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("failed to serve", zap.Error(err))
			s.hc.Set(healthcheck.Broken)
		}
	}()
	wg.Wait() // wait for the server to start listening
	s.logger.Info(
		"Admin server started",
		zap.String("http.host-port", l.Addr().String()),
		zap.Stringer("health-status", s.hc.Get()))
	return nil
}

func (s *AdminServer) registerPprofHandlers() {
	s.mux.HandleFunc("/debug/pprof/", pprof.Index)
	s.mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	s.mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	s.mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	s.mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	s.mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	s.mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	s.mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	s.mux.Handle("/debug/pprof/block", pprof.Handler("block"))
}

// Close stops the HTTP server
func (s *AdminServer) Close() error {
	err := s.server.Shutdown(context.Background())
	s.stopped.Wait()
	return err
}
