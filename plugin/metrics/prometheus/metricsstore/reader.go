// Copyright (c) 2021 The Jaeger Authors.
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

package metricsstore

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/multierror"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

// MetricsReader is a Prometheus metrics reader.
type MetricsReader struct {
	url    string
	logger *zap.Logger
}

// NewMetricsReader returns a new MetricsReader, assigning the first reachable host:port from the provided list.
// This host:port forms part of the URL to call when making queries to the underlying metrics store.
func NewMetricsReader(logger *zap.Logger, hostPorts []string, connTimeout time.Duration) (*MetricsReader, error) {
	if len(hostPorts) < 1 {
		return nil, fmt.Errorf("no prometheus query host:port provided")
	}
	errs := make([]error, 0)
	for _, hostPort := range hostPorts {
		host, port, err := net.SplitHostPort(hostPort)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), connTimeout)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		logger.Info("Connected to Prometheus backend", zap.String("http-addr", hostPort))
		conn.Close()

		return &MetricsReader{
			url:    fmt.Sprintf("http://%s/api/v1/query_range", hostPort),
			logger: logger,
		}, nil
	}
	return nil, fmt.Errorf("none of the provided prometheus query host:ports are reachable: %w", multierror.Wrap(errs))
}

// GetLatencies gets the latency metrics for the given set of latency query parameters.
func (m *MetricsReader) GetLatencies(ctx context.Context, params *metricsstore.LatenciesQueryParameters) ([]metrics.Metric, error) {
	// TODO: Implement me
	return nil, nil
}

// GetCallRates gets the call rate metrics for the given set of call rate query parameters.
func (m *MetricsReader) GetCallRates(ctx context.Context, params *metricsstore.CallRateQueryParameters) ([]metrics.Metric, error) {
	// TODO: Implement me
	return nil, nil
}

// GetErrorRates gets the error rate metrics for the given set of error rate query parameters.
func (m *MetricsReader) GetErrorRates(ctx context.Context, params *metricsstore.ErrorRateQueryParameters) ([]metrics.Metric, error) {
	// TODO: Implement me
	return nil, nil
}

// GetMinStepDuration gets the minimum step duration (the smallest possible duration between two data points in a time series) supported.
func (m *MetricsReader) GetMinStepDuration(_ context.Context, _ *metricsstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return time.Millisecond, nil
}
