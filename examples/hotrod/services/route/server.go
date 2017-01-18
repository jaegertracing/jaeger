// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package route

import (
	"context"
	"encoding/json"
	"math"
	"math/rand"
	"net/http"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/uber-go/zap"

	"github.com/uber/jaeger/examples/hotrod/pkg/delay"
	"github.com/uber/jaeger/examples/hotrod/pkg/httperr"
	"github.com/uber/jaeger/examples/hotrod/pkg/httpexpvar"
	"github.com/uber/jaeger/examples/hotrod/pkg/log"
	"github.com/uber/jaeger/examples/hotrod/pkg/tracing"
	"github.com/uber/jaeger/examples/hotrod/services/config"
)

// Server implements Route service
type Server struct {
	hostPort string
	tracer   opentracing.Tracer
	logger   log.Factory
}

// NewServer creates a new route.Server
func NewServer(hostPort string, tracer opentracing.Tracer, logger log.Factory) *Server {
	return &Server{
		hostPort: hostPort,
		tracer:   tracer,
		logger:   logger,
	}
}

// Run starts the Route server
func (s *Server) Run() error {
	mux := s.createServeMux()
	s.logger.Bg().Info("Starting", zap.String("address", "http://"+s.hostPort))
	return http.ListenAndServe(s.hostPort, mux)
}

func (s *Server) createServeMux() http.Handler {
	mux := tracing.NewServeMux(s.tracer)
	mux.Handle("/route", http.HandlerFunc(s.route))
	mux.Handle("/debug/vars", http.HandlerFunc(httpexpvar.Handler))
	return mux
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.logger.For(ctx).Info("HTTP request received", zap.String("method", r.Method), zap.Object("url", r.URL))
	if err := r.ParseForm(); httperr.HandleError(w, err, http.StatusBadRequest) {
		s.logger.For(ctx).Error("bad request", zap.Error(err))
		return
	}

	pickup := r.Form.Get("pickup")
	if pickup == "" {
		http.Error(w, "Missing required 'pickup' parameter", http.StatusBadRequest)
		return
	}

	dropoff := r.Form.Get("dropoff")
	if dropoff == "" {
		http.Error(w, "Missing required 'dropoff' parameter", http.StatusBadRequest)
		return
	}

	response := computeRoute(ctx, pickup, dropoff)

	data, err := json.Marshal(response)
	if httperr.HandleError(w, err, http.StatusInternalServerError) {
		s.logger.For(ctx).Error("cannot marshal response", zap.Error(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func computeRoute(ctx context.Context, pickup, dropoff string) *Route {
	start := time.Now()
	defer func() {
		updateCalcStats(ctx, time.Since(start))
	}()

	// Simulate expensive calculation
	delay.Sleep(config.RouteCalcDelay, config.RouteCalcDelayStdDev)

	eta := math.Max(2, rand.NormFloat64()*3+5)
	println(eta)
	return &Route{
		Pickup:  pickup,
		Dropoff: dropoff,
		ETA:     time.Duration(eta) * time.Minute,
	}
}
