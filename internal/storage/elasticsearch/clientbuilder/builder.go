// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package clientbuilder

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	esv8 "github.com/elastic/go-elasticsearch/v9"
	"github.com/olivere/elastic/v7"
	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapgrpc"

	"github.com/jaegertracing/jaeger/internal/auth"
	"github.com/jaegertracing/jaeger/internal/headerforwarding"
	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	eswrapper "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/wrapper"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
)

type bulkCallback struct {
	startTimes sync.Map
	sm         *spanstoremetrics.WriteMetrics
	logger     *zap.Logger
}

// NewClient creates a new ElasticSearch client
func NewClient(ctx context.Context, c *config.Configuration, logger *zap.Logger, metricsFactory metrics.Factory, httpAuth extensionauth.HTTPClient) (es.Client, error) {
	if len(c.Servers) < 1 {
		return nil, errors.New("no servers specified")
	}
	options, err := getConfigOptions(ctx, c, logger, httpAuth)
	if err != nil {
		return nil, err
	}

	rawClient, err := elastic.NewClient(options...)
	if err != nil {
		return nil, err
	}

	bcb := bulkCallback{
		sm:     spanstoremetrics.NewWriter(metricsFactory, "bulk_index"),
		logger: logger,
	}

	version := es.BackendVersion(c.Version)
	if version == 0 {
		// Determine backend version
		pingResult, pingStatus, err := rawClient.Ping(c.Servers[0]).Do(ctx)
		if err != nil {
			return nil, err
		}

		// Non-2xx responses aren't reported as errors by the ping code (7.0.32 version of
		// the elastic client).
		if pingStatus < 200 || pingStatus >= 300 {
			return nil, fmt.Errorf("ElasticSearch server %s returned HTTP %d, expected 2xx", c.Servers[0], pingStatus)
		}

		// The deserialization in the ping implementation may succeed even if the response
		// contains no relevant properties and we may get empty values in that case.
		if pingResult.Version.Number == "" {
			return nil, fmt.Errorf("ElasticSearch server %s returned invalid ping response", c.Servers[0])
		}

		majorVersion, err := strconv.Atoi(string(pingResult.Version.Number[0]))
		if err != nil {
			return nil, err
		}
		version = es.DetectBackendVersion(pingResult.TagLine, majorVersion)
		c.Version = uint(version)
		logger.Info("Backend detected", zap.Stringer("version", version))
	}

	var rawClientV8 *esv8.Client
	if version.UsesV8API() {
		rawClientV8, err = newElasticsearchV8(ctx, c, logger, httpAuth)
		if err != nil {
			return nil, fmt.Errorf("error creating v8 client: %w", err)
		}
	}

	bulkProc, err := rawClient.BulkProcessor().
		Before(func(id int64, _ /* requests */ []elastic.BulkableRequest) {
			bcb.startTimes.Store(id, time.Now())
		}).
		After(bcb.invoke).
		BulkSize(c.BulkProcessing.MaxBytes).
		Workers(c.BulkProcessing.Workers).
		BulkActions(c.BulkProcessing.MaxActions).
		FlushInterval(c.BulkProcessing.FlushInterval).
		Do(ctx)
	if err != nil {
		return nil, err
	}

	return eswrapper.WrapESClient(rawClient, bulkProc, version, rawClientV8), nil
}

func (bcb *bulkCallback) invoke(id int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
	start, ok := bcb.startTimes.Load(id)
	if ok {
		bcb.startTimes.Delete(id)
	} else {
		start = time.Now()
	}

	// Log individual errors
	if response != nil && response.Errors {
		for _, it := range response.Items {
			for key, val := range it {
				if val.Error != nil {
					bcb.logger.Error("Elasticsearch part of bulk request failed",
						zap.String("map-key", key), zap.Reflect("response", val))
				}
			}
		}
	}

	latency := time.Since(start.(time.Time))
	if err != nil {
		bcb.sm.LatencyErr.Record(latency)
	} else {
		bcb.sm.LatencyOk.Record(latency)
	}

	var failed int
	if response != nil {
		failed = len(response.Failed())
	}

	total := len(requests)
	bcb.sm.Attempts.Inc(int64(total))
	bcb.sm.Inserts.Inc(int64(total - failed))
	bcb.sm.Errors.Inc(int64(failed))

	if err != nil {
		bcb.logger.Error("Elasticsearch could not process bulk request",
			zap.Int("request_count", total),
			zap.Int("failed_count", failed),
			zap.Error(err),
			zap.Any("response", response))
	}
}

func newElasticsearchV8(ctx context.Context, c *config.Configuration, logger *zap.Logger, httpAuth extensionauth.HTTPClient) (*esv8.Client, error) {
	var options esv8.Config
	options.Addresses = c.Servers
	if c.Authentication.BasicAuthentication.HasValue() {
		basicAuth := c.Authentication.BasicAuthentication.Get()
		options.Username = basicAuth.Username
		options.Password = basicAuth.Password
	}
	options.DiscoverNodesOnStart = c.Sniffing.Enabled
	options.CompressRequestBody = c.HTTPCompression

	if len(c.CustomHeaders) > 0 {
		headers := make(http.Header)
		for key, value := range c.CustomHeaders {
			headers.Set(key, value)
		}
		options.Header = headers
	}

	transport, err := GetHTTPRoundTripper(ctx, c, logger, httpAuth)
	if err != nil {
		return nil, err
	}
	// Outermost wrapper: forward headers captured on the inbound request context
	// (populated by the jaeger_query header_forwarding middleware/interceptors)
	// onto every outbound request to Elasticsearch.
	options.Transport = headerforwarding.NewHTTPClientRoundTripper(transport)
	return esv8.NewClient(options)
}

func getESOptions(c *config.Configuration, disableHealthCheck bool) []elastic.ClientOptionFunc {
	// Get base Elasticsearch options
	options := []elastic.ClientOptionFunc{
		elastic.SetURL(c.Servers...), elastic.SetSniff(c.Sniffing.Enabled), elastic.SetHealthcheck(!disableHealthCheck),
	}
	if c.HealthCheckTimeoutStartup > 0 {
		options = append(options, elastic.SetHealthcheckTimeoutStartup(c.HealthCheckTimeoutStartup))
	}
	if c.Sniffing.UseHTTPS {
		options = append(options, elastic.SetScheme("https"))
	}
	if c.SendGetBodyAs != "" {
		options = append(options, elastic.SetSendGetBodyAs(c.SendGetBodyAs))
	}
	options = append(options, elastic.SetGzip(c.HTTPCompression))
	return options
}

// getConfigOptions wraps the configs to feed to the ElasticSearch client init
func getConfigOptions(ctx context.Context, c *config.Configuration, logger *zap.Logger, httpAuth extensionauth.HTTPClient) ([]elastic.ClientOptionFunc, error) {
	// (has problems on AWS OpenSearch) see https://github.com/jaegertracing/jaeger/pull/7212
	// Disable health check only in the following cases:
	// 1. When health check is explicitly disabled
	// 2. When tokens are EXCLUSIVELY available from context (not from file)
	//    because at startup we don't have a valid token to do the health check
	disableHealthCheck := c.DisableHealthCheck

	// Check if we have bearer token or API key authentication that only allows from context
	if c.Authentication.BearerTokenAuth.HasValue() || c.Authentication.APIKeyAuth.HasValue() {
		bearerAuth := c.Authentication.BearerTokenAuth.Get()
		apiKeyAuth := c.Authentication.APIKeyAuth.Get()

		disableHealthCheck = disableHealthCheck ||
			(bearerAuth != nil && bearerAuth.AllowFromContext && bearerAuth.FilePath == "") ||
			(apiKeyAuth != nil && apiKeyAuth.AllowFromContext && apiKeyAuth.FilePath == "")
	}

	// Get base Elasticsearch options using the helper function
	options := getESOptions(c, disableHealthCheck)
	// Configure HTTP transport with TLS and authentication
	transport, err := GetHTTPRoundTripper(ctx, c, logger, httpAuth)
	if err != nil {
		return nil, err
	}

	// Outermost wrapper: forward headers captured on the inbound request context
	// (populated by the jaeger_query header_forwarding middleware/interceptors)
	// onto every outbound request to Elasticsearch.
	transport = headerforwarding.NewHTTPClientRoundTripper(transport)

	// HTTP client setup with timeout and transport
	httpClient := &http.Client{
		Timeout:   c.QueryTimeout,
		Transport: transport,
	}

	options = append(options, elastic.SetHttpClient(httpClient))

	// Add logging configuration
	options, err = addLoggerOptions(options, c.LogLevel, logger)
	if err != nil {
		return options, err
	}

	return options, nil
}

func addLoggerOptions(options []elastic.ClientOptionFunc, logLevel string, logger *zap.Logger) ([]elastic.ClientOptionFunc, error) {
	// Decouple ES logger from the log-level assigned to the parent application's log-level; otherwise, the least
	// permissive log-level will dominate.
	// e.g. --log-level=info and --es.log-level=debug would mute ES's debug logging and would require --log-level=debug
	// to show ES debug logs.
	var lvl zapcore.Level
	var setLogger func(logger elastic.Logger) elastic.ClientOptionFunc

	switch logLevel {
	case "debug":
		lvl = zap.DebugLevel
		setLogger = elastic.SetTraceLog
	case "info":
		lvl = zap.InfoLevel
		setLogger = elastic.SetInfoLog
	case "error":
		lvl = zap.ErrorLevel
		setLogger = elastic.SetErrorLog
	default:
		return options, fmt.Errorf("unrecognized log-level: \"%s\"", logLevel)
	}

	esLogger := logger.WithOptions(
		zap.IncreaseLevel(lvl),
		zap.AddCallerSkip(2), // to ensure the right caller:lineno are logged
	)

	// Elastic client requires a "Printf"-able logger.
	l := zapgrpc.NewLogger(esLogger)
	options = append(options, setLogger(l))
	return options, nil
}

// getBodyFixRoundTripper ensures req.GetBody is populated when req.Body is set.
// The olivere/elastic v7 client sets req.Body directly without setting GetBody,
// which breaks HTTP authenticators (like sigv4auth) that rely on GetBody to hash
// the request payload for signing.
type getBodyFixRoundTripper struct {
	base http.RoundTripper
}

func (t *getBodyFixRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil && req.GetBody == nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	return t.base.RoundTrip(req)
}

// GetHTTPRoundTripper returns configured http.RoundTripper with optional HTTP authenticator.
// Pass nil for httpAuth if authentication is not required.
func GetHTTPRoundTripper(ctx context.Context, c *config.Configuration, logger *zap.Logger, httpAuth extensionauth.HTTPClient) (http.RoundTripper, error) {
	// Configure base transport.
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	// Configure TLS.
	if c.TLS.Insecure {
		// #nosec G402
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	} else {
		tlsConfig, err := c.TLS.LoadTLSConfig(ctx)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tlsConfig
	}

	// Initialize authentication methods.
	var authMethods []auth.Method
	// API Key Authentication
	if c.Authentication.APIKeyAuth.HasValue() {
		apiKeyAuth := c.Authentication.APIKeyAuth.Get()
		ak, err := initAPIKeyAuth(apiKeyAuth, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize API key authentication: %w", err)
		}
		if ak != nil {
			authMethods = append(authMethods, *ak)
		}
	}

	// Bearer Token Authentication
	if c.Authentication.BearerTokenAuth.HasValue() {
		bearerAuth := c.Authentication.BearerTokenAuth.Get()
		ba, err := initBearerAuth(bearerAuth, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize bearer authentication: %w", err)
		}
		if ba != nil {
			authMethods = append(authMethods, *ba)
		}
	}

	// Basic Authentication
	if c.Authentication.BasicAuthentication.HasValue() {
		basicAuth := c.Authentication.BasicAuthentication.Get()
		ba, err := initBasicAuth(basicAuth, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize basic authentication: %w", err)
		}
		if ba != nil {
			authMethods = append(authMethods, *ba)
		}
	}

	// Wrap with authentication layer.
	var roundTripper http.RoundTripper = transport
	if len(authMethods) > 0 {
		roundTripper = &auth.RoundTripper{
			Transport: transport,
			Auths:     authMethods,
		}
	}

	// Apply HTTP authenticator extension if configured (e.g., SigV4).
	// getBodyFixRoundTripper must wrap the authenticator on the OUTSIDE so it
	// runs first and populates req.GetBody before the authenticator hashes the
	// payload for signing. Authenticators like sigv4auth hash req.GetBody (and
	// substitute the empty-payload hash when it is nil); the olivere/elastic
	// client sets req.Body without req.GetBody, so without this ordering
	// body-bearing requests are signed as empty and rejected (see #8760).
	if httpAuth != nil {
		wrappedRT, err := httpAuth.RoundTripper(roundTripper)
		if err != nil {
			return nil, fmt.Errorf("failed to wrap round tripper with HTTP authenticator: %w", err)
		}
		return &getBodyFixRoundTripper{base: wrappedRT}, nil
	}

	return roundTripper, nil
}
