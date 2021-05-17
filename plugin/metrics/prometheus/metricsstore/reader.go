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
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/api"
	promapi "github.com/prometheus/client_golang/api/prometheus/v1"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

// MetricsReader is a Prometheus metrics reader.
type MetricsReader struct {
	client promapi.API
	logger *zap.Logger
}

// NewMetricsReader returns a new MetricsReader.
func NewMetricsReader(logger *zap.Logger, hostPort string, connectTimeout time.Duration) (*MetricsReader, error) {
	// KeepAlive and TLSHandshake timeouts are kept to existing Prometheus client's
	// DefaultRoundTripper to simplify user configuration and may be made configurable when required.
	roundTripper := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   connectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	client, err := api.NewClient(api.Config{
		Address:      "http://" + hostPort,
		RoundTripper: roundTripper,
	})
	if err != nil {
		return nil, err
	}
	mr := &MetricsReader{
		client: promapi.NewAPI(client),
		logger: logger,
	}
	logger.Info("Prometheus reader initialized", zap.String("addr", hostPort))
	return mr, nil
}

// GetLatencies gets the latency metrics for the given set of latency query parameters.
func (m *MetricsReader) GetLatencies(ctx context.Context, params *metricsstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	// TODO: Implement me
	return &metrics.MetricFamily{}, nil
}

// GetCallRates gets the call rate metrics for the given set of call rate query parameters.
func (m *MetricsReader) GetCallRates(ctx context.Context, params *metricsstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	// TODO: Implement me
	return &metrics.MetricFamily{}, nil
}

// GetErrorRates gets the error rate metrics for the given set of error rate query parameters.
func (m *MetricsReader) GetErrorRates(ctx context.Context, params *metricsstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	// TODO: Implement me
	return &metrics.MetricFamily{}, nil
}

// GetMinStepDuration gets the minimum step duration (the smallest possible duration between two data points in a time series) supported.
func (m *MetricsReader) GetMinStepDuration(_ context.Context, _ *metricsstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return time.Millisecond, nil
}
