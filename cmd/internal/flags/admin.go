// Copyright (c) 2019 The Jaeger Authors.
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

package flags

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
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
	logger               *zap.Logger
	adminHostPort        string
	hc                   *healthcheck.HealthCheck
	mux                  *http.ServeMux
	server               *http.Server
	tlsCfg               *tls.Config
	tlsCertWatcherCloser io.Closer
}

// NewAdminServer creates a new admin server.
func NewAdminServer(hostPort string) *AdminServer {
	return &AdminServer{
		adminHostPort: hostPort,
		logger:        zap.NewNop(),
		hc:            healthcheck.New(),
		mux:           http.NewServeMux(),
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
	flagSet.String(adminHTTPHostPort, s.adminHostPort, fmt.Sprintf("The host:port (e.g. 127.0.0.1%s or %s) for the admin server, including health check, /metrics, etc.", s.adminHostPort, s.adminHostPort))
	tlsAdminHTTPFlagsConfig.AddFlags(flagSet)
}

// InitFromViper initializes the server with properties retrieved from Viper.
func (s *AdminServer) initFromViper(v *viper.Viper, logger *zap.Logger) error {
	s.setLogger(logger)

	s.adminHostPort = v.GetString(adminHTTPHostPort)
	var tlsAdminHTTP tlscfg.Options
	tlsAdminHTTP, err := tlsAdminHTTPFlagsConfig.InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse admin server TLS options: %w", err)
	}
	if tlsAdminHTTP.Enabled {
		tlsCfg, err := tlsAdminHTTP.Config(s.logger) // This checks if the certificates are correctly provided
		if err != nil {
			return err
		}
		s.tlsCfg = tlsCfg
		s.tlsCertWatcherCloser = &tlsAdminHTTP
	} else {
		s.tlsCertWatcherCloser = io.NopCloser(nil)
	}
	return nil
}

// Handle adds a new handler to the admin server.
func (s *AdminServer) Handle(path string, handler http.Handler) {
	s.mux.Handle(path, handler)
}

// Serve starts HTTP server.
func (s *AdminServer) Serve() error {
	l, err := net.Listen("tcp", s.adminHostPort)
	if err != nil {
		s.logger.Error("Admin server failed to listen", zap.Error(err))
		return err
	}
	s.serveWithListener(l)

	s.logger.Info(
		"Admin server started",
		zap.String("http.host-port", l.Addr().String()),
		zap.Stringer("health-status", s.hc.Get()))
	return nil
}

func (s *AdminServer) serveWithListener(l net.Listener) {
	s.logger.Info("Mounting health check on admin server", zap.String("route", "/"))
	s.mux.Handle("/", s.hc.Handler())
	version.RegisterHandler(s.mux, s.logger)
	s.registerPprofHandlers()
	recoveryHandler := recoveryhandler.NewRecoveryHandler(s.logger, true)
	errorLog, _ := zap.NewStdLogAt(s.logger, zapcore.ErrorLevel)
	s.server = &http.Server{
		Handler:           recoveryHandler(s.mux),
		ErrorLog:          errorLog,
		ReadHeaderTimeout: 2 * time.Second,
	}
	if s.tlsCfg != nil {
		s.server.TLSConfig = s.tlsCfg
	}
	s.logger.Info("Starting admin HTTP server", zap.String("http-addr", s.adminHostPort))
	go func() {
		var err error
		if s.tlsCfg != nil {
			err = s.server.ServeTLS(l, "", "")
		} else {
			err = s.server.Serve(l)
		}
		switch err {
		case nil, http.ErrServerClosed:
			// normal exit, nothing to do
		default:
			s.logger.Error("failed to serve", zap.Error(err))
			s.hc.Set(healthcheck.Broken)
		}
	}()
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
	_ = s.tlsCertWatcherCloser.Close()
	return s.server.Shutdown(context.Background())
}
