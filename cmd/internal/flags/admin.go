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

	s.serverCfg.Endpoint = v.GetString(adminHTTPHostPort)
	tlsAdminHTTP, err := tlsAdminHTTPFlagsConfig.InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse admin server TLS options: %w", err)
	}
	tlsAdminHTTPConfig := tlsAdminHTTP.ToOtelServerConfig()
	if tlsAdminHTTPConfig != nil {
		s.serverCfg.TLSSetting = tlsAdminHTTPConfig
		_, err = s.serverCfg.TLSSetting.LoadTLSConfig(context.Background())
		if err != nil {
			return err
		}
	}
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
	if err = s.serveWithListener(l); err != nil {
		return err
	}

	s.logger.Info(
		"Admin server started",
		zap.String("http.host-port", l.Addr().String()),
		zap.Stringer("health-status", s.hc.Get()))
	return nil
}

func (s *AdminServer) serveWithListener(l net.Listener) (err error) {
	s.logger.Info("Mounting health check on admin server", zap.String("route", "/"))
	s.mux.Handle("/", s.hc.Handler())
	version.RegisterHandler(s.mux, s.logger)
	s.registerPprofHandlers()
	recoveryHandler := recoveryhandler.NewRecoveryHandler(s.logger, true)
	errorLog, _ := zap.NewStdLogAt(s.logger, zapcore.ErrorLevel)
	s.server, err = s.serverCfg.ToServer(context.Background(), nil, telemetry.NoopSettings().ToOtelComponent(),
		recoveryHandler(s.mux))
	if err != nil {
		return err
	}
	s.server.ErrorLog = errorLog

	if s.serverCfg.TLSSetting != nil {
		s.server.TLSConfig, err = s.serverCfg.TLSSetting.LoadTLSConfig(context.Background())
		if err != nil {
			return err
		}
	}
	s.logger.Info("Starting admin HTTP server", zap.String("http-addr", s.serverCfg.Endpoint))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := s.server.Serve(l)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("failed to serve", zap.Error(err))
			s.hc.Set(healthcheck.Broken)
		}
	}()

	go func() {
		wg.Wait()
	}()
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
	return errors.Join(
		s.server.Shutdown(context.Background()),
	)
}
