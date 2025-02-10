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
	esV8 "github.com/elastic/go-elasticsearch/v8"
	"github.com/olivere/elastic"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapgrpc"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/es"
	eswrapper "github.com/jaegertracing/jaeger/pkg/es/wrapper"
	"github.com/jaegertracing/jaeger/pkg/metrics"
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
	Replicas int64 `mapstructure:"replicas"`
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
	// SendGetBodyAs is the HTTP verb to use for requests that contain a body.
	SendGetBodyAs string `mapstructure:"send_get_body_as"`
	// QueryTimeout contains the timeout used for queries. A timeout of zero means no timeout.
	QueryTimeout time.Duration `mapstructure:"query_timeout"`

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

type Authentication struct {
	BasicAuthentication       BasicAuthentication       `mapstructure:"basic"`
	BearerTokenAuthentication BearerTokenAuthentication `mapstructure:"bearer_token"`
}

type BasicAuthentication struct {
	// Username contains the username required to connect to Elasticsearch.
	Username string `mapstructure:"username"`
	// Password contains The password required by Elasticsearch
	Password string `mapstructure:"password" json:"-"`
	// PasswordFilePath contains the path to a file containing password.
	// This file is watched for changes.
	PasswordFilePath string `mapstructure:"password_file"`
}

// BearerTokenAuthentication contains the configuration for attaching bearer tokens
// when making HTTP requests. Note that TokenFilePath and AllowTokenFromContext
// should not both be enabled. If both TokenFilePath and AllowTokenFromContext are set,
// the TokenFilePath will be ignored.
// For more information about token-based authentication in elasticsearch, check out
// https://www.elastic.co/guide/en/elasticsearch/reference/current/token-authentication-services.html.
type BearerTokenAuthentication struct {
	// FilePath contains the path to a file containing a bearer token.
	FilePath string `mapstructure:"file_path"`
	// AllowTokenFromContext, if set to true, enables reading bearer token from the context.
	AllowFromContext bool `mapstructure:"from_context"`
}

// NewClient creates a new ElasticSearch client
func NewClient(c *Configuration, logger *zap.Logger, metricsFactory metrics.Factory) (es.Client, error) {
	if len(c.Servers) < 1 {
		return nil, errors.New("no servers specified")
	}
	options, err := c.getConfigOptions(logger)
	if err != nil {
		return nil, err
	}

	rawClient, err := elastic.NewClient(options...)
	if err != nil {
		return nil, err
	}

	sm := spanstoremetrics.NewWriter(metricsFactory, "bulk_index")
	m := sync.Map{}

	bulkProc, err := rawClient.BulkProcessor().
		Before(func(id int64, _ /* requests */ []elastic.BulkableRequest) {
			m.Store(id, time.Now())
		}).
		After(func(id int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
			start, ok := m.Load(id)
			if !ok {
				return
			}
			m.Delete(id)

			// log individual errors, note that err might be false and these errors still present
			if response != nil && response.Errors {
				for _, it := range response.Items {
					for key, val := range it {
						if val.Error != nil {
							logger.Error("Elasticsearch part of bulk request failed", zap.String("map-key", key),
								zap.Reflect("response", val))
						}
					}
				}
			}

			sm.Emit(err, time.Since(start.(time.Time)))
			if err != nil {
				var failed int
				if response == nil {
					failed = 0
				} else {
					failed = len(response.Failed())
				}
				total := len(requests)
				logger.Error("Elasticsearch could not process bulk request",
					zap.Int("request_count", total),
					zap.Int("failed_count", failed),
					zap.Error(err),
					zap.Any("response", response))
			}
		}).
		BulkSize(c.BulkProcessing.MaxBytes).
		Workers(c.BulkProcessing.Workers).
		BulkActions(c.BulkProcessing.MaxActions).
		FlushInterval(c.BulkProcessing.FlushInterval).
		Do(context.Background())
	if err != nil {
		return nil, err
	}

	if c.Version == 0 {
		// Determine ElasticSearch Version
		pingResult, _, err := rawClient.Ping(c.Servers[0]).Do(context.Background())
		if err != nil {
			return nil, err
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
		}
		logger.Info("Elasticsearch detected", zap.Int("version", esVersion))
		//nolint: gosec // G115
		c.Version = uint(esVersion)
	}

	var rawClientV8 *esV8.Client
	if c.Version >= 8 {
		rawClientV8, err = newElasticsearchV8(c, logger)
		if err != nil {
			return nil, fmt.Errorf("error creating v8 client: %w", err)
		}
	}

	return eswrapper.WrapESClient(rawClient, bulkProc, c.Version, rawClientV8), nil
}

func newElasticsearchV8(c *Configuration, logger *zap.Logger) (*esV8.Client, error) {
	var options esV8.Config
	options.Addresses = c.Servers
	options.Username = c.Authentication.BasicAuthentication.Username
	options.Password = c.Authentication.BasicAuthentication.Password
	options.DiscoverNodesOnStart = c.Sniffing.Enabled
	transport, err := GetHTTPRoundTripper(c, logger)
	if err != nil {
		return nil, err
	}
	options.Transport = transport
	return esV8.NewClient(options)
}

func setDefaultIndexOptions(target, source *IndexOptions) {
	if target.Shards == 0 {
		target.Shards = source.Shards
	}

	if target.Replicas == 0 {
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
	if c.Authentication.BasicAuthentication.Username == "" {
		c.Authentication.BasicAuthentication.Username = source.Authentication.BasicAuthentication.Username
	}
	if c.Authentication.BasicAuthentication.Password == "" {
		c.Authentication.BasicAuthentication.Password = source.Authentication.BasicAuthentication.Password
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

// getConfigOptions wraps the configs to feed to the ElasticSearch client init
func (c *Configuration) getConfigOptions(logger *zap.Logger) ([]elastic.ClientOptionFunc, error) {
	options := []elastic.ClientOptionFunc{
		elastic.SetURL(c.Servers...), elastic.SetSniff(c.Sniffing.Enabled),
		// Disable health check when token from context is allowed, this is because at this time
		// we don'r have a valid token to do the check ad if we don't disable the check the service that
		// uses this won't start.
		elastic.SetHealthcheck(!c.Authentication.BearerTokenAuthentication.AllowFromContext),
	}
	if c.Sniffing.UseHTTPS {
		options = append(options, elastic.SetScheme("https"))
	}
	httpClient := &http.Client{
		Timeout: c.QueryTimeout,
	}
	options = append(options, elastic.SetHttpClient(httpClient))

	if c.Authentication.BasicAuthentication.Password != "" && c.Authentication.BasicAuthentication.PasswordFilePath != "" {
		return nil, errors.New("both Password and PasswordFilePath are set")
	}
	if c.Authentication.BasicAuthentication.PasswordFilePath != "" {
		passwordFromFile, err := loadTokenFromFile(c.Authentication.BasicAuthentication.PasswordFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load password from file: %w", err)
		}
		c.Authentication.BasicAuthentication.Password = passwordFromFile
	}
	options = append(options, elastic.SetBasicAuth(c.Authentication.BasicAuthentication.Username, c.Authentication.BasicAuthentication.Password))

	if c.SendGetBodyAs != "" {
		options = append(options, elastic.SetSendGetBodyAs(c.SendGetBodyAs))
	}

	options, err := addLoggerOptions(options, c.LogLevel, logger)
	if err != nil {
		return options, err
	}

	transport, err := GetHTTPRoundTripper(c, logger)
	if err != nil {
		return nil, err
	}
	httpClient.Transport = transport
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

// GetHTTPRoundTripper returns configured http.RoundTripper
func GetHTTPRoundTripper(c *Configuration, logger *zap.Logger) (http.RoundTripper, error) {
	if !c.TLS.Insecure {
		ctlsConfig, err := c.TLS.LoadTLSConfig(context.Background())
		if err != nil {
			return nil, err
		}
		return &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: ctlsConfig,
		}, nil
	}
	var transport http.RoundTripper
	httpTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		// #nosec G402
		TLSClientConfig: &tls.Config{InsecureSkipVerify: c.TLS.InsecureSkipVerify},
	}
	if c.TLS.CAFile != "" {
		ctlsConfig, err := c.TLS.LoadTLSConfig(context.Background())
		if err != nil {
			return nil, err
		}
		httpTransport.TLSClientConfig = ctlsConfig
		transport = httpTransport
	}

	token := ""
	if c.Authentication.BearerTokenAuthentication.FilePath != "" {
		if c.Authentication.BearerTokenAuthentication.AllowFromContext {
			logger.Warn("Token file and token propagation are both enabled, token from file won't be used")
		}
		tokenFromFile, err := loadTokenFromFile(c.Authentication.BearerTokenAuthentication.FilePath)
		if err != nil {
			return nil, err
		}
		token = tokenFromFile
	}
	if token != "" || c.Authentication.BearerTokenAuthentication.AllowFromContext {
		transport = bearertoken.RoundTripper{
			Transport:       httpTransport,
			OverrideFromCtx: c.Authentication.BearerTokenAuthentication.AllowFromContext,
			StaticToken:     token,
		}
	}
	return transport, nil
}

func loadTokenFromFile(path string) (string, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

func (c *Configuration) Validate() error {
	_, err := govalidator.ValidateStruct(c)
	return err
}
