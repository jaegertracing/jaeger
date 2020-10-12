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
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
	"github.com/jaegertracing/jaeger/pkg/version"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	healthCheckHTTPPort = "health-check-http-port"
	adminHTTPPort       = "admin-http-port"
	adminHTTPHostPort   = "admin.http.host-port"

	healthCheckHTTPPortWarning = "(deprecated, will be removed after 2020-03-15 or in release v1.19.0, whichever is later)"
	adminHTTPPortWarning       = "(deprecated, will be removed after 2020-06-30 or in release v1.20.0, whichever is later)"
)

// AdminServer runs an HTTP server with admin endpoints, such as healthcheck at /, /metrics, etc.
type AdminServer struct {
	logger        *zap.Logger
	adminHostPort string

	hc *healthcheck.HealthCheck

	mux    *http.ServeMux
	server *http.Server
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
	flagSet.Int(healthCheckHTTPPort, 0, healthCheckHTTPPortWarning+" see --"+adminHTTPHostPort)
	flagSet.Int(adminHTTPPort, 0, adminHTTPPortWarning+" see --"+adminHTTPHostPort)
	flagSet.String(adminHTTPHostPort, s.adminHostPort, fmt.Sprintf("The host:port (e.g. 127.0.0.1%s or %s) for the admin server, including health check, /metrics, etc.", s.adminHostPort, s.adminHostPort))
}

// Util function to use deprecated flag value if specified
func (s *AdminServer) checkDeprecatedFlag(v *viper.Viper, actualFlagName string, expectedFlagName string) {
	if v := v.GetInt(actualFlagName); v != 0 {
		s.logger.Sugar().Warnf("Using deprecated flag %s, please upgrade to %s", actualFlagName, expectedFlagName)
		s.adminHostPort = ports.PortToHostPort(v)
	}
}

// InitFromViper initializes the server with properties retrieved from Viper.
func (s *AdminServer) initFromViper(v *viper.Viper, logger *zap.Logger) {
	s.setLogger(logger)

	s.adminHostPort = v.GetString(adminHTTPHostPort)
	s.checkDeprecatedFlag(v, healthCheckHTTPPort, adminHTTPHostPort)
	s.checkDeprecatedFlag(v, adminHTTPPort, adminHTTPHostPort)
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
	s.server = &http.Server{Handler: recoveryHandler(s.mux)}
	s.logger.Info("Starting admin HTTP server", zap.String("http-addr", s.adminHostPort))
	go func() {
		switch err := s.server.Serve(l); err {
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
	return s.server.Shutdown(context.Background())
}
