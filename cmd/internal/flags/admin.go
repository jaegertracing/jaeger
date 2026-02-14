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
	"go.opentelemetry.io/collector/config/confignet"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jaegertracing/jaeger/internal/config/tlscfg"
	"github.com/jaegertracing/jaeger/internal/recoveryhandler"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/version"
)

const (
	adminHTTPHostPort = "admin.http.host-port"
)

var tlsAdminHTTPFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "admin.http",
}

// AdminServer runs an HTTP server with admin endpoints, such as /metrics, /debug/pprof, health check, etc.
type AdminServer struct {
	logger    *zap.Logger
	mux       *http.ServeMux
	server    *http.Server
	serverCfg confighttp.ServerConfig
	stopped   sync.WaitGroup
	hc        *HealthHost
}

// NewAdminServer creates a new admin server.
func NewAdminServer(hostPort string) *AdminServer {
	return &AdminServer{
		logger: zap.NewNop(),
		mux:    http.NewServeMux(),
		serverCfg: confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  hostPort,
				Transport: confignet.TransportTypeTCP,
			},
		},
		hc: NewHealthHost(),
	}
}

// Host returns the health host for this admin server.
// It implements component.Host and componentstatus.Reporter,
// allowing it to be used with telemetry.Settings and componentstatus.ReportStatus.
func (s *AdminServer) Host() *HealthHost {
	return s.hc
}

// setLogger initializes logger.
func (s *AdminServer) setLogger(logger *zap.Logger) {
	s.logger = logger
}

// AddFlags registers CLI flags.
func (s *AdminServer) AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(adminHTTPHostPort, s.serverCfg.NetAddr.Endpoint, fmt.Sprintf("The host:port (e.g. 127.0.0.1%s or %s) for the admin server, including health check, /metrics, etc.", s.serverCfg.NetAddr.Endpoint, s.serverCfg.NetAddr.Endpoint))
	tlsAdminHTTPFlagsConfig.AddFlags(flagSet)
}

// InitFromViper initializes the server with properties retrieved from Viper.
func (s *AdminServer) initFromViper(v *viper.Viper, logger *zap.Logger) error {
	s.setLogger(logger)

	tlsAdminHTTP, err := tlsAdminHTTPFlagsConfig.InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse admin server TLS options: %w", err)
	}

	s.serverCfg.NetAddr.Endpoint = v.GetString(adminHTTPHostPort)
	s.serverCfg.TLS = tlsAdminHTTP
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
	//nolint:revive // not the same as wg.Go() which would call Done() on exit, not on start
	wg.Add(1)
	s.stopped.Add(1)
	go func() {
		wg.Done()
		defer s.stopped.Done()
		err := s.server.Serve(l)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("failed to serve", zap.Error(err))
			s.hc.SetUnavailable()
		}
	}()
	wg.Wait() // wait for the server to start listening
	s.logger.Info("Admin server started", zap.String("http.host-port", l.Addr().String()))
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
