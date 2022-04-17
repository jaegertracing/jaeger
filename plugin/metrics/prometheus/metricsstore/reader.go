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
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/opentracing/opentracing-go"
	ottag "github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/prometheus/client_golang/api"
	promapi "github.com/prometheus/client_golang/api/prometheus/v1"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/prometheus/config"
	"github.com/jaegertracing/jaeger/plugin/metrics/prometheus/metricsstore/dbmodel"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

const (
	minStep = time.Millisecond

	latenciesMetricName = "service_latencies"
	latenciesMetricDesc = "%.2fth quantile latency, grouped by service"

	callsMetricName = "service_call_rate"
	callsMetricDesc = "calls/sec, grouped by service"

	errorsMetricName = "service_error_rate"
	errorsMetricDesc = "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service"
)

type (
	// MetricsReader is a Prometheus metrics reader.
	MetricsReader struct {
		client promapi.API
		logger *zap.Logger
	}

	promQueryParams struct {
		groupBy        string
		spanKindFilter string
		serviceFilter  string
		rate           string
	}

	metricsQueryParams struct {
		metricsstore.BaseQueryParameters
		groupByHistBucket bool
		metricName        string
		metricDesc        string
		buildPromQuery    func(p promQueryParams) string
	}
)

// NewMetricsReader returns a new MetricsReader.
func NewMetricsReader(logger *zap.Logger, cfg config.Configuration) (*MetricsReader, error) {
	roundTripper, err := getHTTPRoundTripper(&cfg, logger)
	if err != nil {
		return nil, err
	}
	client, err := api.NewClient(api.Config{
		Address:      cfg.ServerURL,
		RoundTripper: roundTripper,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize prometheus client: %w", err)
	}
	mr := &MetricsReader{
		client: promapi.NewAPI(client),
		logger: logger,
	}
	logger.Info("Prometheus reader initialized", zap.String("addr", cfg.ServerURL))
	return mr, nil
}

// GetLatencies gets the latency metrics for the given set of latency query parameters.
func (m *MetricsReader) GetLatencies(ctx context.Context, requestParams *metricsstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	metricsParams := metricsQueryParams{
		BaseQueryParameters: requestParams.BaseQueryParameters,
		groupByHistBucket:   true,
		metricName:          latenciesMetricName,
		metricDesc:          fmt.Sprintf(latenciesMetricDesc, requestParams.Quantile),
		buildPromQuery: func(p promQueryParams) string {
			return fmt.Sprintf(
				// Note: p.spanKindFilter can be ""; trailing commas are okay within a timeseries selection.
				`histogram_quantile(%.2f, sum(rate(latency_bucket{service_name =~ "%s", %s}[%s])) by (%s))`,
				requestParams.Quantile,
				p.serviceFilter,
				p.spanKindFilter,
				p.rate,
				p.groupBy,
			)
		},
	}
	return m.executeQuery(ctx, metricsParams)
}

// GetCallRates gets the call rate metrics for the given set of call rate query parameters.
func (m *MetricsReader) GetCallRates(ctx context.Context, requestParams *metricsstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	metricsParams := metricsQueryParams{
		BaseQueryParameters: requestParams.BaseQueryParameters,
		metricName:          callsMetricName,
		metricDesc:          callsMetricDesc,
		buildPromQuery: func(p promQueryParams) string {
			return fmt.Sprintf(
				// Note: p.spanKindFilter can be ""; trailing commas are okay within a timeseries selection.
				`sum(rate(calls_total{service_name =~ "%s", %s}[%s])) by (%s)`,
				p.serviceFilter,
				p.spanKindFilter,
				p.rate,
				p.groupBy,
			)
		},
	}
	return m.executeQuery(ctx, metricsParams)
}

// GetErrorRates gets the error rate metrics for the given set of error rate query parameters.
func (m *MetricsReader) GetErrorRates(ctx context.Context, requestParams *metricsstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	metricsParams := metricsQueryParams{
		BaseQueryParameters: requestParams.BaseQueryParameters,
		metricName:          errorsMetricName,
		metricDesc:          errorsMetricDesc,
		buildPromQuery: func(p promQueryParams) string {
			return fmt.Sprintf(
				// Note: p.spanKindFilter can be ""; trailing commas are okay within a timeseries selection.
				`sum(rate(calls_total{service_name =~ "%s", status_code = "STATUS_CODE_ERROR", %s}[%s])) by (%s) / sum(rate(calls_total{service_name =~ "%s", %s}[%s])) by (%s)`,
				p.serviceFilter, p.spanKindFilter, p.rate, p.groupBy,
				p.serviceFilter, p.spanKindFilter, p.rate, p.groupBy,
			)
		},
	}
	return m.executeQuery(ctx, metricsParams)
}

// GetMinStepDuration gets the minimum step duration (the smallest possible duration between two data points in a time series) supported.
func (m *MetricsReader) GetMinStepDuration(_ context.Context, _ *metricsstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return minStep, nil
}

// executeQuery executes a query against a Prometheus-compliant metrics backend.
func (m *MetricsReader) executeQuery(ctx context.Context, p metricsQueryParams) (*metrics.MetricFamily, error) {
	if p.GroupByOperation {
		p.metricName = strings.Replace(p.metricName, "service", "service_operation", 1)
		p.metricDesc += " & operation"
	}
	promQuery := buildPromQuery(p)

	span, ctx := startSpanForQuery(ctx, p.metricName, promQuery)
	defer span.Finish()

	queryRange := promapi.Range{
		Start: p.EndTime.Add(-1 * *p.Lookback),
		End:   *p.EndTime,
		Step:  *p.Step,
	}

	m.logger.Debug("Executing Prometheus query", zap.String("query", promQuery), zap.Any("range", queryRange))

	mv, warnings, err := m.client.QueryRange(ctx, promQuery, queryRange)
	if err != nil {
		logErrorToSpan(span, err)
		return &metrics.MetricFamily{}, fmt.Errorf("failed executing metrics query: %w", err)
	}
	if len(warnings) > 0 {
		m.logger.Warn("Warnings detected on Prometheus query", zap.Any("warnings", warnings))
	}

	m.logger.Debug("Prometheus query results", zap.String("results", mv.String()))
	return dbmodel.ToDomainMetricsFamily(
		p.metricName,
		p.metricDesc,
		mv,
	)
}

func buildPromQuery(metricsParams metricsQueryParams) string {
	groupBy := []string{"service_name"}
	if metricsParams.GroupByOperation {
		groupBy = append(groupBy, "operation")
	}
	if metricsParams.groupByHistBucket {
		// Group by the bucket value ("le" => "less than or equal to").
		groupBy = append(groupBy, "le")
	}

	spanKindFilter := ""
	if len(metricsParams.SpanKinds) > 0 {
		spanKindFilter = fmt.Sprintf(`span_kind =~ "%s"`, strings.Join(metricsParams.SpanKinds, "|"))
	}
	promParams := promQueryParams{
		serviceFilter:  strings.Join(metricsParams.ServiceNames, "|"),
		spanKindFilter: spanKindFilter,
		rate:           promqlDurationString(metricsParams.RatePer),
		groupBy:        strings.Join(groupBy, ","),
	}
	return metricsParams.buildPromQuery(promParams)
}

// promqlDurationString formats the duration string to be promQL-compliant.
// PromQL only accepts "single-unit" durations like "30s", "1m", "1h"; not "1h5s" or "1m0s".
func promqlDurationString(d *time.Duration) string {
	var b []byte
	for _, c := range d.String() {
		b = append(b, byte(c))
		if unicode.IsLetter(c) {
			break
		}
	}
	return string(b)
}

func startSpanForQuery(ctx context.Context, metricName, query string) (opentracing.Span, context.Context) {
	span, ctx := opentracing.StartSpanFromContext(ctx, metricName)
	ottag.DBStatement.Set(span, query)
	ottag.DBType.Set(span, "prometheus")
	ottag.Component.Set(span, "promql")
	return span, ctx
}

func logErrorToSpan(span opentracing.Span, err error) {
	ottag.Error.Set(span, true)
	span.LogFields(otlog.Error(err))
}

func getHTTPRoundTripper(c *config.Configuration, logger *zap.Logger) (rt http.RoundTripper, err error) {
	var ctlsConfig *tls.Config
	if c.TLS.Enabled {
		if ctlsConfig, err = c.TLS.Config(logger); err != nil {
			return nil, err
		}
	}

	// KeepAlive and TLSHandshake timeouts are kept to existing Prometheus client's
	// DefaultRoundTripper to simplify user configuration and may be made configurable when required.
	httpTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   c.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     ctlsConfig,
	}
	return bearertoken.RoundTripper{
		Transport:       httpTransport,
		OverrideFromCtx: true,
	}, nil
}
