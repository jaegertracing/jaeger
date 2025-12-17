// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/asaskevich/govalidator"
	esv8 "github.com/elastic/go-elasticsearch/v9"
	"github.com/olivere/elastic/v7"
	"go.opentelemetry.io/collector/config/configauth"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapgrpc"

	"github.com/jaegertracing/jaeger/internal/auth"
	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	eswrapper "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/wrapper"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
)

const (
	IndexPrefixSeparator = "-"
)

// IndexOptions describes the index format and rollover frequency
type IndexOptions struct {
	// Priority contains the priority of index template (ESv8 only).
	Priority int64 `mapstructure:"priority"`
	// DateLayout contains the format string used to format current time to part of the index name.
	// For example, "2006-01-02" layout will result in "jaeger-spans-yyyy-mm-dd".
	// If not specified, the default value is "2006-01-02".
	// See https://pkg.go.dev/time#Layout for more details on the syntax.
	DateLayout string `mapstructure:"date_layout"`
	// Shards is the number of shards per index in Elasticsearch.
	Shards int64 `mapstructure:"shards"`
	// Replicas is the number of replicas per index in Elasticsearch.
	Replicas *int64 `mapstructure:"replicas"`
	// RolloverFrequency contains the rollover frequency setting used to fetch
	// indices from elasticsearch.
	// Valid configuration options are: [hour, day].
	// This setting does not affect the index rotation and is simply used for
	// fetching indices.
	RolloverFrequency string `mapstructure:"rollover_frequency"`
}

// Indices describes different configuration options for each index type
type Indices struct {
	// IndexPrefix is an optional prefix to prepend to Jaeger indices.
	// For example, setting this field to "production" creates "production-jaeger-*".
	IndexPrefix  IndexPrefix  `mapstructure:"index_prefix"`
	Spans        IndexOptions `mapstructure:"spans"`
	Services     IndexOptions `mapstructure:"services"`
	Dependencies IndexOptions `mapstructure:"dependencies"`
	Sampling     IndexOptions `mapstructure:"sampling"`
}

type bulkCallback struct {
	startTimes sync.Map
	sm         *spanstoremetrics.WriteMetrics
	logger     *zap.Logger
}

type IndexPrefix string

func (p IndexPrefix) Apply(indexName string) string {
	ps := string(p)
	if ps == "" {
		return indexName
	}
	if strings.HasSuffix(ps, IndexPrefixSeparator) {
		return ps + indexName
	}
	return ps + IndexPrefixSeparator + indexName
}

// Configuration describes the configuration properties needed to connect to an ElasticSearch cluster
type Configuration struct {
	// ---- connection related configs ----
	// Servers is a list of Elasticsearch servers. The strings must must contain full URLs
	// (i.e. http://localhost:9200).
	Servers []string `mapstructure:"server_urls" valid:"required,url"`
	// RemoteReadClusters is a list of Elasticsearch remote cluster names for cross-cluster
	// querying.
	RemoteReadClusters []string       `mapstructure:"remote_read_clusters"`
	Authentication     Authentication `mapstructure:"auth"`
	// TLS contains the TLS configuration for the connection to the ElasticSearch clusters.
	TLS      configtls.ClientConfig `mapstructure:"tls"`
	Sniffing Sniffing               `mapstructure:"sniffing"`
	// Disable the Elasticsearch health check
	DisableHealthCheck bool `mapstructure:"disable_health_check"`
	// SendGetBodyAs is the HTTP verb to use for requests that contain a body.
	SendGetBodyAs string `mapstructure:"send_get_body_as"`
	// QueryTimeout contains the timeout used for queries. A timeout of zero means no timeout.
	QueryTimeout time.Duration `mapstructure:"query_timeout"`
	// HTTPCompression can be set to false to disable gzip compression for requests to ElasticSearch
	HTTPCompression bool `mapstructure:"http_compression"`

	// CustomHeaders contains custom HTTP headers to be sent with every request to Elasticsearch.
	// This is useful for scenarios like AWS SigV4 proxy authentication where specific headers
	// (like Host) need to be set for proper request signing.
	CustomHeaders map[string]string `mapstructure:"custom_headers"`
	// ---- elasticsearch client related configs ----
	BulkProcessing BulkProcessing `mapstructure:"bulk_processing"`
	// Version contains the major Elasticsearch version. If this field is not specified,
	// the value will be auto-detected from Elasticsearch.
	Version uint `mapstructure:"version"`
	// LogLevel contains the Elasticsearch client log-level. Valid values for this field
	// are: [debug, info, error]
	LogLevel string `mapstructure:"log_level"`

	// ---- index related configs ----
	Indices Indices `mapstructure:"indices"`
	// UseReadWriteAliases, if set to true, will use read and write aliases for indices.
	// Use this option with Elasticsearch rollover API. It requires an external component
	// to create aliases before startup and then performing its management.
	UseReadWriteAliases bool `mapstructure:"use_aliases"`
	// SpanReadAlias specifies the exact alias name to use for reading spans.
	// When set, Jaeger will use this alias directly without any modifications.
	// This allows integration with existing Elasticsearch setups that have custom alias names.
	// Can only be used with UseReadWriteAliases=true.
	// Example: "my-custom-span-reader"
	SpanReadAlias string `mapstructure:"span_read_alias"`
	// SpanWriteAlias specifies the exact alias name to use for writing spans.
	// When set, Jaeger will use this alias directly without any modifications.
	// Can only be used with UseReadWriteAliases=true.
	// Example: "my-custom-span-writer"
	SpanWriteAlias string `mapstructure:"span_write_alias"`
	// ServiceReadAlias specifies the exact alias name to use for reading services.
	// When set, Jaeger will use this alias directly without any modifications.
	// Can only be used with UseReadWriteAliases=true.
	// Example: "my-custom-service-reader"
	ServiceReadAlias string `mapstructure:"service_read_alias"`
	// ServiceWriteAlias specifies the exact alias name to use for writing services.
	// When set, Jaeger will use this alias directly without any modifications.
	// Can only be used with UseReadWriteAliases=true.
	// Example: "my-custom-service-writer"
	ServiceWriteAlias string `mapstructure:"service_write_alias"`
	// ReadAliasSuffix is the suffix to append to the index name used for reading.
	// This configuration only exists to provide backwards compatibility for jaeger-v1
	// which is why it is not exposed as a configuration option for jaeger-v2
	ReadAliasSuffix string `mapstructure:"-"`
	// WriteAliasSuffix is the suffix to append to the write index name.
	// This configuration only exists to provide backwards compatibility for jaeger-v1
	// which is why it is not exposed as a configuration option for jaeger-v2
	WriteAliasSuffix string `mapstructure:"-"`
	// CreateIndexTemplates, if set to true, creates index templates at application startup.
	// This configuration should be set to false when templates are installed manually.
	CreateIndexTemplates bool `mapstructure:"create_mappings"`
	// Option to enable Index Lifecycle Management (ILM) for Jaeger span and service indices.
	// Read more about ILM at
	// https://www.jaegertracing.io/docs/deployment/#enabling-ilm-support
	UseILM bool `mapstructure:"use_ilm"`

	// ---- jaeger-specific configs ----
	// MaxDocCount Defines maximum number of results to fetch from storage per query.
	MaxDocCount int `mapstructure:"max_doc_count"`
	// MaxSpanAge configures the maximum lookback on span reads.
	MaxSpanAge time.Duration `mapstructure:"max_span_age"`
	// ServiceCacheTTL contains the TTL for the cache of known service names.
	ServiceCacheTTL time.Duration `mapstructure:"service_cache_ttl"`
	// AdaptiveSamplingLookback contains the duration to look back for the
	// latest adaptive sampling probabilities.
	AdaptiveSamplingLookback time.Duration `mapstructure:"adaptive_sampling_lookback"`
	Tags                     TagsAsFields  `mapstructure:"tags_as_fields"`
	// Enabled, if set to true, enables the namespace for storage pointed to by this configuration.
	Enabled bool `mapstructure:"-"`
}

// TagsAsFields holds configuration for tag schema.
// By default Jaeger stores tags in an array of nested objects.
// This configurations allows to store tags as object fields for better Kibana support.
type TagsAsFields struct {
	// Store all tags as object fields, instead nested objects
	AllAsFields bool `mapstructure:"all"`
	// Dot replacement for tag keys when stored as object fields
	DotReplacement string `mapstructure:"dot_replacement"`
	// File path to tag keys which should be stored as object fields
	File string `mapstructure:"config_file"`
	// Comma delimited list of tags to store as object fields
	Include string `mapstructure:"include"`
}

// Sniffing sets the sniffing configuration for the ElasticSearch client, which is the process
// of finding all the nodes of your cluster. Read more about sniffing at
// https://github.com/olivere/elastic/wiki/Sniffing.
type Sniffing struct {
	// Enabled, if set to true, enables sniffing for the ElasticSearch client.
	Enabled bool `mapstructure:"enabled"`
	// UseHTTPS, if set to true, sets the HTTP scheme to HTTPS when performing sniffing.
	// For ESV8, the scheme is set to HTTPS by default, so this configuration is ignored.
	UseHTTPS bool `mapstructure:"use_https"`
}

type BulkProcessing struct {
	// MaxBytes, contains the number of bytes which specifies when to flush.
	MaxBytes int `mapstructure:"max_bytes"`
	// MaxActions contain the number of added actions which specifies when to flush.
	MaxActions int `mapstructure:"max_actions"`
	// FlushInterval is the interval at the end of which a flush occurs.
	FlushInterval time.Duration `mapstructure:"flush_interval"`
	// Workers contains the number of concurrent workers allowed to be executed.
	Workers int `mapstructure:"workers"`
}

// TokenAuthentication contains the common fields shared by all token-based authentication methods
type TokenAuthentication struct {
	// FilePath contains the path to a file containing the token.
	FilePath string `mapstructure:"file_path"`
	// AllowFromContext, if set to true, allows the token to be retrieved from the context.
	AllowFromContext bool `mapstructure:"from_context"`
	// ReloadInterval contains the interval at which the token file is reloaded.
	// If set to 0 then the file is only loaded once on startup.
	ReloadInterval time.Duration `mapstructure:"reload_interval"`
}

type Authentication struct {
	BasicAuthentication configoptional.Optional[BasicAuthentication] `mapstructure:"basic"`
	BearerTokenAuth     configoptional.Optional[TokenAuthentication] `mapstructure:"bearer_token"`
	APIKeyAuth          configoptional.Optional[TokenAuthentication] `mapstructure:"api_key"`
	configauth.Config   `mapstructure:",squash"`
}

type BasicAuthentication struct {
	// Username contains the username required to connect to Elasticsearch.
	Username string `mapstructure:"username"`
	// Password contains The password required by Elasticsearch
	Password string `mapstructure:"password" json:"-"`
	// PasswordFilePath contains the path to a file containing password.
	// This file is watched for changes.
	PasswordFilePath string `mapstructure:"password_file"`
	// ReloadInterval contains the interval at which the password file is reloaded.
	// If set to 0 then the file is only loaded once on startup.
	ReloadInterval time.Duration `mapstructure:"reload_interval"`
}

// BearerTokenAuthentication contains the configuration for attaching bearer tokens
// when making HTTP requests. Note that TokenFilePath and AllowTokenFromContext
// should not both be enabled. If both TokenFilePath and AllowTokenFromContext are set,
// the TokenFilePath will be ignored.
// For more information about token-based authentication in elasticsearch, check out
// https://www.elastic.co/guide/en/elasticsearch/reference/current/token-authentication-services.html.

// NewClient creates a new ElasticSearch client
func NewClient(ctx context.Context, c *Configuration, logger *zap.Logger, metricsFactory metrics.Factory, httpAuth extensionauth.HTTPClient) (es.Client, error) {
	if len(c.Servers) < 1 {
		return nil, errors.New("no servers specified")
	}
	options, err := c.getConfigOptions(ctx, logger, httpAuth)
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

	if c.Version == 0 {
		// Determine ElasticSearch Version
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

		esVersion, err := strconv.Atoi(string(pingResult.Version.Number[0]))
		if err != nil {
			return nil, err
		}
		// OpenSearch is based on ES 7.x
		if strings.Contains(pingResult.TagLine, "OpenSearch") {
			if pingResult.Version.Number[0] == '1' {
				logger.Info("OpenSearch 1.x detected, using ES 7.x index mappings")
				esVersion = 7
			}
			if pingResult.Version.Number[0] == '2' {
				logger.Info("OpenSearch 2.x detected, using ES 7.x index mappings")
				esVersion = 7
			}
			if pingResult.Version.Number[0] == '3' {
				logger.Info("OpenSearch 3.x detected, using ES 7.x index mappings")
				esVersion = 7
			}
		}
		logger.Info("Elasticsearch detected", zap.Int("version", esVersion))
		//nolint:gosec // G115
		c.Version = uint(esVersion)
	}

	var rawClientV8 *esv8.Client
	if c.Version >= 8 {
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

	return eswrapper.WrapESClient(rawClient, bulkProc, c.Version, rawClientV8), nil
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

func newElasticsearchV8(ctx context.Context, c *Configuration, logger *zap.Logger, httpAuth extensionauth.HTTPClient) (*esv8.Client, error) {
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
	options.Transport = transport
	return esv8.NewClient(options)
}

func setDefaultIndexOptions(target, source *IndexOptions) {
	if target.Shards == 0 {
		target.Shards = source.Shards
	}

	if target.Replicas == nil {
		target.Replicas = source.Replicas
	}

	if target.Priority == 0 {
		target.Priority = source.Priority
	}

	if target.DateLayout == "" {
		target.DateLayout = source.DateLayout
	}

	if target.RolloverFrequency == "" {
		target.RolloverFrequency = source.RolloverFrequency
	}
}

// ApplyDefaults copies settings from source unless its own value is non-zero.
func (c *Configuration) ApplyDefaults(source *Configuration) {
	if len(c.RemoteReadClusters) == 0 {
		c.RemoteReadClusters = source.RemoteReadClusters
	}
	// Handle BasicAuthentication defaults
	sourceHasBasicAuth := source.Authentication.BasicAuthentication.HasValue()
	targetHasBasicAuth := c.Authentication.BasicAuthentication.HasValue()
	if sourceHasBasicAuth {
		// If target doesn't have BasicAuth, copy it from source
		if !targetHasBasicAuth {
			c.Authentication.BasicAuthentication = source.Authentication.BasicAuthentication
		} else {
			// Target has BasicAuth, apply field-level defaults
			sourceBasicAuth := source.Authentication.BasicAuthentication.Get()
			// Make a copy of target BasicAuth
			basicAuth := *c.Authentication.BasicAuthentication.Get()

			// Apply defaults for username if not set
			if basicAuth.Username == "" && sourceBasicAuth.Username != "" {
				basicAuth.Username = sourceBasicAuth.Username
			}
			// Apply defaults for password if not set
			if basicAuth.Password == "" && sourceBasicAuth.Password != "" {
				basicAuth.Password = sourceBasicAuth.Password
			}

			// Only update BasicAuthentication if we have values to set
			if basicAuth.Username != "" || basicAuth.Password != "" {
				c.Authentication.BasicAuthentication = configoptional.Some(basicAuth)
			}
		}
	}
	if !c.Sniffing.Enabled {
		c.Sniffing.Enabled = source.Sniffing.Enabled
	}
	if c.MaxSpanAge == 0 {
		c.MaxSpanAge = source.MaxSpanAge
	}
	if c.AdaptiveSamplingLookback == 0 {
		c.AdaptiveSamplingLookback = source.AdaptiveSamplingLookback
	}
	if c.Indices.IndexPrefix == "" {
		c.Indices.IndexPrefix = source.Indices.IndexPrefix
	}

	setDefaultIndexOptions(&c.Indices.Spans, &source.Indices.Spans)
	setDefaultIndexOptions(&c.Indices.Services, &source.Indices.Services)
	setDefaultIndexOptions(&c.Indices.Dependencies, &source.Indices.Dependencies)

	if c.BulkProcessing.MaxBytes == 0 {
		c.BulkProcessing.MaxBytes = source.BulkProcessing.MaxBytes
	}
	if c.BulkProcessing.Workers == 0 {
		c.BulkProcessing.Workers = source.BulkProcessing.Workers
	}
	if c.BulkProcessing.MaxActions == 0 {
		c.BulkProcessing.MaxActions = source.BulkProcessing.MaxActions
	}
	if c.BulkProcessing.FlushInterval == 0 {
		c.BulkProcessing.FlushInterval = source.BulkProcessing.FlushInterval
	}
	if !c.Sniffing.UseHTTPS {
		c.Sniffing.UseHTTPS = source.Sniffing.UseHTTPS
	}
	if !c.Tags.AllAsFields {
		c.Tags.AllAsFields = source.Tags.AllAsFields
	}
	if c.Tags.DotReplacement == "" {
		c.Tags.DotReplacement = source.Tags.DotReplacement
	}
	if c.Tags.Include == "" {
		c.Tags.Include = source.Tags.Include
	}
	if c.Tags.File == "" {
		c.Tags.File = source.Tags.File
	}
	if c.MaxDocCount == 0 {
		c.MaxDocCount = source.MaxDocCount
	}
	if c.LogLevel == "" {
		c.LogLevel = source.LogLevel
	}
	if c.SendGetBodyAs == "" {
		c.SendGetBodyAs = source.SendGetBodyAs
	}
	if !c.HTTPCompression {
		c.HTTPCompression = source.HTTPCompression
	}
	if c.CustomHeaders == nil && len(source.CustomHeaders) > 0 {
		c.CustomHeaders = make(map[string]string)
		for k, v := range source.CustomHeaders {
			c.CustomHeaders[k] = v
		}
	}
}

// RolloverFrequencyAsNegativeDuration returns the index rollover frequency duration for the given frequency string
func RolloverFrequencyAsNegativeDuration(frequency string) time.Duration {
	if frequency == "hour" {
		return -1 * time.Hour
	}
	return -24 * time.Hour
}

// TagKeysAsFields returns tags from the file and command line merged
func (c *Configuration) TagKeysAsFields() ([]string, error) {
	var tags []string

	// from file
	if c.Tags.File != "" {
		file, err := os.Open(filepath.Clean(c.Tags.File))
		if err != nil {
			return nil, err
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if tag := strings.TrimSpace(line); tag != "" {
				tags = append(tags, tag)
			}
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
	}

	// from params
	if c.Tags.Include != "" {
		tags = append(tags, strings.Split(c.Tags.Include, ",")...)
	}

	return tags, nil
}

func (c *Configuration) getESOptions(disableHealthCheck bool) []elastic.ClientOptionFunc {
	// Get base Elasticsearch options
	options := []elastic.ClientOptionFunc{
		elastic.SetURL(c.Servers...), elastic.SetSniff(c.Sniffing.Enabled), elastic.SetHealthcheck(!disableHealthCheck),
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
func (c *Configuration) getConfigOptions(ctx context.Context, logger *zap.Logger, httpAuth extensionauth.HTTPClient) ([]elastic.ClientOptionFunc, error) {
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
	options := c.getESOptions(disableHealthCheck)
	// Configure HTTP transport with TLS and authentication
	transport, err := GetHTTPRoundTripper(ctx, c, logger, httpAuth)
	if err != nil {
		return nil, err
	}

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

// GetHTTPRoundTripper returns configured http.RoundTripper with optional HTTP authenticator.
// Pass nil for httpAuth if authentication is not required.
func GetHTTPRoundTripper(ctx context.Context, c *Configuration, logger *zap.Logger, httpAuth extensionauth.HTTPClient) (http.RoundTripper, error) {
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

	// Apply HTTP authenticator extension if configured (e.g., SigV4)
	if httpAuth != nil {
		wrappedRT, err := httpAuth.RoundTripper(roundTripper)
		if err != nil {
			return nil, fmt.Errorf("failed to wrap round tripper with HTTP authenticator: %w", err)
		}
		return wrappedRT, nil
	}

	return roundTripper, nil
}

func (c *Configuration) Validate() error {
	_, err := govalidator.ValidateStruct(c)
	if err != nil {
		return err
	}
	if c.UseILM && !c.UseReadWriteAliases {
		return errors.New("UseILM must always be used in conjunction with UseReadWriteAliases to ensure ES writers and readers refer to the single index mapping")
	}
	if c.CreateIndexTemplates && c.UseILM {
		return errors.New("when UseILM is set true, CreateIndexTemplates must be set to false and index templates must be created by init process of es-rollover app")
	}

	// Validate explicit alias settings require UseReadWriteAliases
	hasAnyExplicitAlias := c.SpanReadAlias != "" || c.SpanWriteAlias != "" ||
		c.ServiceReadAlias != "" || c.ServiceWriteAlias != ""

	if hasAnyExplicitAlias && !c.UseReadWriteAliases {
		return errors.New("explicit aliases (span_read_alias, span_write_alias, service_read_alias, service_write_alias) require UseReadWriteAliases to be true")
	}

	// Validate that if any alias is set, all four should be set (for consistency)
	hasSpanAliases := c.SpanReadAlias != "" || c.SpanWriteAlias != ""
	hasServiceAliases := c.ServiceReadAlias != "" || c.ServiceWriteAlias != ""

	if hasSpanAliases && (c.SpanReadAlias == "" || c.SpanWriteAlias == "") {
		return errors.New("both span_read_alias and span_write_alias must be set together")
	}

	if hasServiceAliases && (c.ServiceReadAlias == "" || c.ServiceWriteAlias == "") {
		return errors.New("both service_read_alias and service_write_alias must be set together")
	}

	return nil
}
