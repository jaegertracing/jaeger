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
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/prometheus/client_golang/api"
	promapi "github.com/prometheus/client_golang/api/prometheus/v1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/prometheus/config"
	"github.com/jaegertracing/jaeger/plugin/metrics/prometheus/metricsstore/dbmodel"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

const (
	minStep = time.Millisecond
)

type (
	// MetricsReader is a Prometheus metrics reader.
	MetricsReader struct {
		client promapi.API
		logger *zap.Logger
		tracer trace.Tracer

		metricsTranslator dbmodel.Translator
		latencyMetricName string
		callsMetricName   string
		operationLabel    string
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
func NewMetricsReader(cfg config.Configuration, logger *zap.Logger, tracer trace.TracerProvider) (*MetricsReader, error) {
	logger.Info("Creating metrics reader", zap.Any("configuration", cfg))

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

	operationLabel := "operation"
	if cfg.SupportSpanmetricsConnector {
		operationLabel = "span_name"
	}

	mr := &MetricsReader{
		client: promapi.NewAPI(client),
		logger: logger,
		tracer: tracer.Tracer("prom-metrics-reader"),

		metricsTranslator: dbmodel.New(operationLabel),
		callsMetricName:   buildFullCallsMetricName(cfg),
		latencyMetricName: buildFullLatencyMetricName(cfg),
		operationLabel:    operationLabel,
	}

	logger.Info("Prometheus reader initialized", zap.String("addr", cfg.ServerURL))
	return mr, nil
}

// GetLatencies gets the latency metrics for the given set of latency query parameters.
func (m MetricsReader) GetLatencies(ctx context.Context, requestParams *metricsstore.LatenciesQueryParameters) (*metrics.MetricFamily, error) {
	metricsParams := metricsQueryParams{
		BaseQueryParameters: requestParams.BaseQueryParameters,
		groupByHistBucket:   true,
		metricName:          "service_latencies",
		metricDesc:          fmt.Sprintf("%.2fth quantile latency, grouped by service", requestParams.Quantile),
		buildPromQuery: func(p promQueryParams) string {
			return fmt.Sprintf(
				// Note: p.spanKindFilter can be ""; trailing commas are okay within a timeseries selection.
				`histogram_quantile(%.2f, sum(rate(%s_bucket{service_name =~ "%s", %s}[%s])) by (%s))`,
				requestParams.Quantile,
				m.latencyMetricName,
				p.serviceFilter,
				p.spanKindFilter,
				p.rate,
				p.groupBy,
			)
		},
	}
	return m.executeQuery(ctx, metricsParams)
}

func buildFullLatencyMetricName(cfg config.Configuration) string {
	metricName := "latency"
	if !cfg.SupportSpanmetricsConnector {
		return metricName
	}
	metricName = "duration"

	if cfg.MetricNamespace != "" {
		metricName = cfg.MetricNamespace + "_" + metricName
	}

	if !cfg.NormalizeDuration {
		return metricName
	}

	// The long names are automatically appended to the metric name by OTEL's prometheus exporters and are defined in:
	//   https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/translator/prometheus#metric-name
	shortToLongName := map[string]string{"ms": "milliseconds", "s": "seconds"}
	lname, ok := shortToLongName[cfg.LatencyUnit]
	if !ok {
		panic("programming error: unknown latency unit: " + cfg.LatencyUnit)
	}
	return metricName + "_" + lname
}

// GetCallRates gets the call rate metrics for the given set of call rate query parameters.
func (m MetricsReader) GetCallRates(ctx context.Context, requestParams *metricsstore.CallRateQueryParameters) (*metrics.MetricFamily, error) {
	metricsParams := metricsQueryParams{
		BaseQueryParameters: requestParams.BaseQueryParameters,
		metricName:          "service_call_rate",
		metricDesc:          "calls/sec, grouped by service",
		buildPromQuery: func(p promQueryParams) string {
			return fmt.Sprintf(
				// Note: p.spanKindFilter can be ""; trailing commas are okay within a timeseries selection.
				`sum(rate(%s{service_name =~ "%s", %s}[%s])) by (%s)`,
				m.callsMetricName,
				p.serviceFilter,
				p.spanKindFilter,
				p.rate,
				p.groupBy,
			)
		},
	}
	return m.executeQuery(ctx, metricsParams)
}

func buildFullCallsMetricName(cfg config.Configuration) string {
	metricName := "calls"
	if !cfg.SupportSpanmetricsConnector {
		return metricName
	}

	if cfg.MetricNamespace != "" {
		metricName = cfg.MetricNamespace + "_" + metricName
	}

	if !cfg.NormalizeCalls {
		return metricName
	}

	return metricName + "_total"
}

// GetErrorRates gets the error rate metrics for the given set of error rate query parameters.
func (m MetricsReader) GetErrorRates(ctx context.Context, requestParams *metricsstore.ErrorRateQueryParameters) (*metrics.MetricFamily, error) {
	metricsParams := metricsQueryParams{
		BaseQueryParameters: requestParams.BaseQueryParameters,
		metricName:          "service_error_rate",
		metricDesc:          "error rate, computed as a fraction of errors/sec over calls/sec, grouped by service",
		buildPromQuery: func(p promQueryParams) string {
			return fmt.Sprintf(
				// Note: p.spanKindFilter can be ""; trailing commas are okay within a timeseries selection.
				`sum(rate(%s{service_name =~ "%s", status_code = "STATUS_CODE_ERROR", %s}[%s])) by (%s) / sum(rate(%s{service_name =~ "%s", %s}[%s])) by (%s)`,
				m.callsMetricName, p.serviceFilter, p.spanKindFilter, p.rate, p.groupBy,
				m.callsMetricName, p.serviceFilter, p.spanKindFilter, p.rate, p.groupBy,
			)
		},
	}
	return m.executeQuery(ctx, metricsParams)
}

// GetMinStepDuration gets the minimum step duration (the smallest possible duration between two data points in a time series) supported.
func (m MetricsReader) GetMinStepDuration(_ context.Context, _ *metricsstore.MinStepDurationQueryParameters) (time.Duration, error) {
	return minStep, nil
}

// executeQuery executes a query against a Prometheus-compliant metrics backend.
func (m MetricsReader) executeQuery(ctx context.Context, p metricsQueryParams) (*metrics.MetricFamily, error) {
	if p.GroupByOperation {
		p.metricName = strings.Replace(p.metricName, "service", "service_operation", 1)
		p.metricDesc += " & operation"
	}
	promQuery := m.buildPromQuery(p)

	ctx, span := startSpanForQuery(ctx, p.metricName, promQuery, m.tracer)
	defer span.End()

	queryRange := promapi.Range{
		Start: p.EndTime.Add(-1 * *p.Lookback),
		End:   *p.EndTime,
		Step:  *p.Step,
	}

	mv, warnings, err := m.client.QueryRange(ctx, promQuery, queryRange)
	if err != nil {
		err = fmt.Errorf("failed executing metrics query: %w", err)
		logErrorToSpan(span, err)
		return &metrics.MetricFamily{}, err
	}
	if len(warnings) > 0 {
		m.logger.Warn("Warnings detected on Prometheus query", zap.Any("warnings", warnings), zap.String("query", promQuery), zap.Any("range", queryRange))
	}

	m.logger.Debug("Prometheus query results", zap.String("results", mv.String()), zap.String("query", promQuery), zap.Any("range", queryRange))

	return m.metricsTranslator.ToDomainMetricsFamily(
		p.metricName,
		p.metricDesc,
		mv,
	)
}

func (m MetricsReader) buildPromQuery(metricsParams metricsQueryParams) string {
	groupBy := []string{"service_name"}
	if metricsParams.GroupByOperation {
		groupBy = append(groupBy, m.operationLabel)
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

func startSpanForQuery(ctx context.Context, metricName, query string, tp trace.Tracer) (context.Context, trace.Span) {
	ctx, span := tp.Start(ctx, metricName)
	span.SetAttributes(
		attribute.Key(semconv.DBStatementKey).String(query),
		attribute.Key(semconv.DBSystemKey).String("prometheus"),
		attribute.Key("component").String("promql"),
	)
	return ctx, span
}

func logErrorToSpan(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
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

	token := ""
	if c.TokenFilePath != "" {
		tokenFromFile, err := loadToken(c.TokenFilePath)
		if err != nil {
			return nil, err
		}
		token = tokenFromFile
	}
	return bearertoken.RoundTripper{
		Transport:       httpTransport,
		OverrideFromCtx: true,
		StaticToken:     token,
	}, nil
}

func loadToken(path string) (string, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("failed to get token from file: %w", err)
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}
