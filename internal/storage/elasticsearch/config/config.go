// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bufio"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/config/configauth"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

const (
	IndexSeparator = "-"

	SpanIndexName       = "jaeger-span"
	ServiceIndexName    = "jaeger-service"
	DependencyIndexName = "jaeger-dependencies"
	SamplingIndexName   = "jaeger-sampling"
)

// WriteMode selects how the Elasticsearch/OpenSearch trace writer persists spans.
type WriteMode string

const (
	// WriteModeAsync enqueues spans into a client-side bulk buffer and returns from
	// WriteTraces before the data is durable. It is the default when write_mode is unset.
	WriteModeAsync WriteMode = "async"
	// WriteModeSync writes each batch with a single blocking _bulk request and returns
	// a real error from WriteTraces if any span fails to persist.
	WriteModeSync WriteMode = "sync"
)

// IndexOptions describes the index format and rollover frequency
type IndexOptions struct {
	// Priority contains the priority of index template (ESv8 only).
	Priority int64 `mapstructure:"priority"`
	// DateLayout contains the format string used to format current time to part of the index name.
	// For example, "2006-01-02" layout will result in "jaeger-spans-yyyy-mm-dd".
	// If not specified, the default value is "2006-01-02".
	// See https://pkg.go.dev/time#Layout for more details on the syntax.
	//
	// Deprecated: superseded by rotation.periodic.date_layout.
	DateLayout configoptional.Optional[string] `mapstructure:"date_layout"`
	// Shards is the number of shards per index in Elasticsearch.
	Shards int64 `mapstructure:"shards"`
	// Replicas is the number of replicas per index in Elasticsearch.
	Replicas *int64 `mapstructure:"replicas"`
	// RolloverFrequency contains the rollover frequency setting used to fetch
	// indices from elasticsearch.
	// Valid configuration options are: [hour, day, month, year].
	// This setting does not affect the index rotation and is simply used for
	// fetching indices.
	//
	// Deprecated: superseded by rotation.periodic.rollover_frequency.
	RolloverFrequency configoptional.Optional[string] `mapstructure:"rollover_frequency"`
	// Rotation defines the index rotation strategy for this index type.
	Rotation RotationConfig `mapstructure:"rotation"`
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

// GetDateLayout returns the effective DateLayout value, defaulting based on
// GetRolloverFrequency() rather than a fixed daily layout.
func (o *IndexOptions) GetDateLayout() string {
	if p := o.DateLayout.Get(); p != nil {
		return *p
	}
	return DefaultDateLayout(o.GetRolloverFrequency())
}

// DefaultDateLayout returns the date-suffix layout matching the granularity
// of the given rollover frequency.
func DefaultDateLayout(frequency string) string {
	switch frequency {
	case "hour":
		return "2006-01-02-15"
	case "month":
		return "2006-01"
	case "year":
		return "2006"
	default:
		return "2006-01-02"
	}
}

// GetRolloverFrequency returns the effective RolloverFrequency value, defaulting to "day".
func (o *IndexOptions) GetRolloverFrequency() string {
	if p := o.RolloverFrequency.Get(); p != nil {
		return *p
	}
	return "day"
}

func (p IndexPrefix) Apply(indexName string) string {
	return joinPrefix(string(p), IndexSeparator, indexName)
}

// DataStreamName is the dot-notation counterpart of Apply for data streams
// (e.g. "prod" -> "prod.jaeger.spans"). A trailing "-" or "." on the prefix is
// dropped so "prod", "prod-" and "prod." all resolve to the same name.
func (p IndexPrefix) DataStreamName(base string) string {
	ps := strings.TrimRight(string(p), IndexSeparator+".")
	return joinPrefix(ps, ".", base)
}

// joinPrefix joins prefix and name with separator, avoiding a doubled separator
// when the prefix already ends with one.
func joinPrefix(prefix, separator, name string) string {
	if prefix == "" {
		return name
	}
	if strings.HasSuffix(prefix, separator) {
		return prefix + name
	}
	return prefix + separator + name
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
	// DisableHealthCheck used to disable the Elasticsearch health check.
	//
	// Deprecated: the owned esclient transport performs no client-side health
	// check, so this setting has no effect since v2.20.0. It is now
	// rejected by config validation and will be removed in a future release.
	DisableHealthCheck configoptional.Optional[bool] `mapstructure:"disable_health_check"`
	// HealthCheckTimeoutStartup used to set the Elasticsearch health check startup timeout.
	//
	// Deprecated: the owned esclient transport performs no client-side health
	// check, so this setting has no effect since v2.20.0. It is now
	// rejected by config validation and will be removed in a future release.
	HealthCheckTimeoutStartup configoptional.Optional[time.Duration] `mapstructure:"health_check_timeout_startup"`
	// SendGetBodyAs used to select the HTTP verb for requests that carry a body.
	//
	// Deprecated: the owned esclient transport sends each request with a fixed
	// verb, so this setting has no effect since v2.20.0. It is now
	// rejected by config validation and will be removed in a future release.
	SendGetBodyAs configoptional.Optional[string] `mapstructure:"send_get_body_as"`
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
	// WriteMode selects how the trace writer persists spans. Valid values are
	// "async" and "sync":
	//   - "async": spans are enqueued into a client-side bulk buffer and
	//     WriteTraces returns before the data is durable (higher throughput, but
	//     a bulk-flush failure is not surfaced to the caller).
	//   - "sync": each batch is written with a single blocking _bulk request and
	//     WriteTraces returns a real error if any span fails to persist,
	//     respecting the tracestore.Writer contract. See RFC 0007 for background.
	// When empty, it defaults to "async". See config.EffectiveWriteMode().
	WriteMode WriteMode `mapstructure:"write_mode"`
	// Version contains the backend version number (e.g. 7, 8, 9 for Elasticsearch,
	// 101, 102, 103 for OpenSearch). If 0, it will be auto-detected from the server.
	Version uint `mapstructure:"version"`
	// LogLevel contains the Elasticsearch client log-level. Valid values for this field
	// are: [debug, info, error]
	LogLevel string `mapstructure:"log_level"`

	// ---- index related configs ----
	Indices Indices `mapstructure:"indices"`

	// UseReadWriteAliases, if set to true, will use read and write aliases for indices.
	// Use this option with Elasticsearch rollover API. It requires an external component
	// to create aliases before startup and then performing its management.
	//
	// Deprecated: superseded by indices.<type>.rotation.manual_rollover or auto_rollover.
	UseReadWriteAliases configoptional.Optional[bool] `mapstructure:"use_aliases"`
	// SpanReadAlias specifies the exact alias name to use for reading spans.
	// When set, Jaeger will use this alias directly without any modifications.
	// This allows integration with existing Elasticsearch setups that have custom alias names.
	// Can only be used with UseReadWriteAliases=true.
	// Example: "my-custom-span-reader"
	//
	// Deprecated: superseded by indices.spans.rotation.manual_rollover.read_alias.
	SpanReadAlias configoptional.Optional[string] `mapstructure:"span_read_alias"`
	// SpanWriteAlias specifies the exact alias name to use for writing spans.
	// When set, Jaeger will use this alias directly without any modifications.
	// Can only be used with UseReadWriteAliases=true.
	// Example: "my-custom-span-writer"
	//
	// Deprecated: superseded by indices.spans.rotation.manual_rollover.write_alias.
	SpanWriteAlias configoptional.Optional[string] `mapstructure:"span_write_alias"`
	// ServiceReadAlias specifies the exact alias name to use for reading services.
	// When set, Jaeger will use this alias directly without any modifications.
	// Can only be used with UseReadWriteAliases=true.
	// Example: "my-custom-service-reader"
	//
	// Deprecated: superseded by indices.services.rotation.manual_rollover.read_alias.
	ServiceReadAlias configoptional.Optional[string] `mapstructure:"service_read_alias"`
	// ServiceWriteAlias specifies the exact alias name to use for writing services.
	// When set, Jaeger will use this alias directly without any modifications.
	// Can only be used with UseReadWriteAliases=true.
	// Example: "my-custom-service-writer"
	//
	// Deprecated: superseded by indices.services.rotation.manual_rollover.write_alias.
	ServiceWriteAlias configoptional.Optional[string] `mapstructure:"service_write_alias"`
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
	// UseILM enables Index Lifecycle Management (ILM) for Jaeger span and service indices.
	// Read more about ILM at
	// https://www.elastic.co/guide/en/elasticsearch/reference/current/index-lifecycle-management.html
	//
	// Deprecated: superseded by indices.<type>.rotation.auto_rollover.
	UseILM configoptional.Optional[bool] `mapstructure:"use_ilm"`

	// ---- jaeger-specific configs ----
	// MaxDocCount Defines maximum number of results to fetch from storage per query.
	MaxDocCount int `mapstructure:"max_doc_count"`
	// MaxSpanAge configures the maximum lookback on span reads.
	// For alias-based rotation (manual_rollover/auto_rollover), this should be set
	// to match the ILM/ISM data retention policy so that GetTraces can find traces
	// up to that age.
	MaxSpanAge time.Duration `mapstructure:"max_span_age"`
	// MaxTraceDuration is the maximum expected duration of a single trace
	// (time between the earliest and latest span in the trace).
	// Used to widen time-range filters when reading spans, ensuring that all spans
	// of a trace are found even if they extend beyond the search window.
	// Defaults to 24h.
	MaxTraceDuration time.Duration `mapstructure:"max_trace_duration"`
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
// of discovering all the nodes of a cluster by querying one of its members.
type Sniffing struct {
	// Enabled, if set to true, enables sniffing for the ElasticSearch client: the
	// client queries one seed node once at startup and adds the cluster's other
	// nodes to its connection pool. Left off by default because a cluster that
	// publishes addresses the client cannot reach (a common AWS/proxy setup)
	// would break; enable it only when every node is directly reachable.
	Enabled bool `mapstructure:"enabled"`
	// UseHTTPS used to force the HTTPS scheme when sniffing discovered nodes.
	//
	// Deprecated: the owned esclient transport derives the scheme of discovered
	// nodes from the seed server URL (an https:// seed already yields https://
	// nodes), so this setting has no effect since v2.20.0. It is now
	// rejected by config validation and will be removed in a future release.
	UseHTTPS configoptional.Optional[bool] `mapstructure:"use_https"`
}

type BulkProcessing struct {
	// MaxBytes, contains the number of bytes which specifies when to flush.
	MaxBytes int `mapstructure:"max_bytes"`
	// MaxActions used to contain the number of added actions which specifies when to flush.
	//
	// Deprecated: the bulk indexer flushes only on a byte threshold (max_bytes) or a
	// flush interval (flush_interval); it has no action-count trigger, so this setting
	// has no effect since v2.20.0. It is now rejected by config validation and will be
	// removed in a future release.
	MaxActions configoptional.Optional[int] `mapstructure:"max_actions"`
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
	// The file is re-read periodically according to ReloadInterval.
	PasswordFilePath string `mapstructure:"password_file"`
	// ReloadInterval is how often the password file is re-read.
	// Defaults to 0, which means the file is read once at startup and never reloaded.
	ReloadInterval time.Duration `mapstructure:"reload_interval"`
}

// BearerTokenAuthentication contains the configuration for attaching bearer tokens
// when making HTTP requests. Note that TokenFilePath and AllowTokenFromContext
// should not both be enabled. If both TokenFilePath and AllowTokenFromContext are set,
// the TokenFilePath will be ignored.
// For more information about token-based authentication in elasticsearch, check out
// https://www.elastic.co/guide/en/elasticsearch/reference/current/token-authentication-services.html.

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

	if !target.DateLayout.HasValue() && source.DateLayout.HasValue() {
		target.DateLayout = source.DateLayout
	}

	if !target.RolloverFrequency.HasValue() && source.RolloverFrequency.HasValue() {
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
	if c.MaxTraceDuration == 0 {
		c.MaxTraceDuration = source.MaxTraceDuration
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
	if c.BulkProcessing.FlushInterval == 0 {
		c.BulkProcessing.FlushInterval = source.BulkProcessing.FlushInterval
	}
	if c.WriteMode == "" {
		c.WriteMode = source.WriteMode
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
	if !c.HTTPCompression {
		c.HTTPCompression = source.HTTPCompression
	}
	if c.CustomHeaders == nil && len(source.CustomHeaders) > 0 {
		c.CustomHeaders = make(map[string]string)
		maps.Copy(c.CustomHeaders, source.CustomHeaders)
	}
}

// RolloverFrequencyAsNegativeDuration returns the index rollover frequency as a negative duration.
func RolloverFrequencyAsNegativeDuration(frequency string) time.Duration {
	return -RolloverFrequencyDuration(frequency)
}

// RolloverFrequencyDuration returns the index rollover frequency as a positive duration.
//
// This value is only used as a step size while scanning backwards from the end of a
// time range to enumerate the indices that need to be queried (see timeRangeIndices).
// Because "month" and "year" do not have a fixed length, an underestimate is returned
// for them (28 days and 365 days respectively) so that scanning never steps over an
// index; at worst it results in one extra, harmless iteration.
func RolloverFrequencyDuration(frequency string) time.Duration {
	switch frequency {
	case "hour":
		return time.Hour
	case "month":
		return 28 * 24 * time.Hour
	case "year":
		return 365 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
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

func (c *Configuration) Validate() error {
	_, err := govalidator.ValidateStruct(c)
	if err != nil {
		return err
	}

	// A non-zero Version is an explicit backend override; reject unsupported
	// values so they don't silently become an Unknown version. 0 means auto-detect.
	if c.Version != 0 && !es.IsSupportedVersion(c.Version) {
		return fmt.Errorf("unsupported version %d: set 0 to auto-detect, or use 7/8/9 (Elasticsearch) or 101/102/103 (OpenSearch 1/2/3)", c.Version)
	}

	// Ensure at most one auth method is configured (they all set the Authorization header).
	var authCount int
	if c.Authentication.BasicAuthentication.HasValue() {
		authCount++
	}
	if c.Authentication.BearerTokenAuth.HasValue() {
		authCount++
	}
	if c.Authentication.APIKeyAuth.HasValue() {
		authCount++
	}
	if authCount > 1 {
		return errors.New("at most one authentication method (basic, bearer_token, api_key) may be configured; all three use the Authorization header")
	}

	// Reject options orphaned when the olivere client stack was retired (#8982):
	// the owned esclient transport never wired them back up, so they have no
	// effect. sniffing.use_https is a sniffing sub-option, disable_health_check /
	// health_check_timeout_startup / send_get_body_as are connection options, and
	// bulk_processing.max_actions is a client option; none has a lever on the
	// current transport. Fail fast rather than accept a setting that silently does
	// nothing.
	if c.Sniffing.UseHTTPS.HasValue() {
		return rejectUnwiredKey("sniffing.use_https",
			"the client derives the scheme of discovered nodes from the seed server URL, "+
				"so an https:// entry in 'server_urls' already yields https:// nodes")
	}
	if c.DisableHealthCheck.HasValue() {
		return rejectUnwiredKey("disable_health_check",
			"the client performs no client-side health check, so there is nothing to disable")
	}
	if c.HealthCheckTimeoutStartup.HasValue() {
		return rejectUnwiredKey("health_check_timeout_startup",
			"the client performs no client-side health check")
	}
	if c.SendGetBodyAs.HasValue() {
		return rejectUnwiredKey("send_get_body_as",
			"the client sends each request with a fixed HTTP verb")
	}
	if c.BulkProcessing.MaxActions.HasValue() {
		return rejectUnwiredKey("bulk_processing.max_actions",
			"the bulk indexer flushes only on a byte threshold ('bulk_processing.max_bytes') "+
				"or a time interval ('bulk_processing.flush_interval'), so an action count has no effect")
	}

	// Validate rotation config for each index type
	if err := c.validateRotationConfig(); err != nil {
		return err
	}

	if RejectLegacyRotationFlags.IsEnabled() && c.hasAnyLegacyRotationFlags() {
		return fmt.Errorf(
			"deprecated ES rotation flags (%s) "+
				"are no longer supported; migrate to 'indices.<type>.rotation' config "+
				"(see https://github.com/jaegertracing/jaeger/pull/8823); "+
				"to temporarily disable this check, use --feature-gates=-jaeger.es.config.rejectLegacyRotationFlags",
			legacyRotationFlagsList,
		)
	}

	if c.getUseILM() && !c.getUseReadWriteAliases() {
		return errors.New("UseILM must always be used in conjunction with UseReadWriteAliases to ensure ES writers and readers refer to the single index mapping")
	}
	if c.CreateIndexTemplates && c.getUseILM() {
		return errors.New("when UseILM is set true, CreateIndexTemplates must be set to false and index templates must be created by init process of es-rollover app")
	}

	hasAnyExplicitAlias := c.getSpanReadAlias() != "" || c.getSpanWriteAlias() != "" ||
		c.getServiceReadAlias() != "" || c.getServiceWriteAlias() != ""

	if hasAnyExplicitAlias && !c.getUseReadWriteAliases() {
		return errors.New("explicit aliases (span_read_alias, span_write_alias, service_read_alias, service_write_alias) require UseReadWriteAliases to be true")
	}

	hasSpanAliases := c.getSpanReadAlias() != "" || c.getSpanWriteAlias() != ""
	hasServiceAliases := c.getServiceReadAlias() != "" || c.getServiceWriteAlias() != ""

	if hasSpanAliases && (c.getSpanReadAlias() == "" || c.getSpanWriteAlias() == "") {
		return errors.New("both span_read_alias and span_write_alias must be set together")
	}

	if hasServiceAliases && (c.getServiceReadAlias() == "" || c.getServiceWriteAlias() == "") {
		return errors.New("both service_read_alias and service_write_alias must be set together")
	}

	if err := validateWriteMode(c.WriteMode); err != nil {
		return err
	}

	return validateLogLevel(c.LogLevel)
}

// EffectiveWriteMode resolves the write mode Jaeger should use: the explicit
// WriteMode from config, or WriteModeAsync when it is unset.
func (c *Configuration) EffectiveWriteMode() WriteMode {
	if c.WriteMode != "" {
		return c.WriteMode
	}
	return WriteModeAsync
}

// validateWriteMode rejects an unrecognized write_mode. An empty value is allowed
// and resolves to the default (WriteModeAsync). It mirrors validateLogLevel:
// write_mode carries no govalidator struct tag, so the whole-config Validate must
// check it explicitly.
func validateWriteMode(mode WriteMode) error {
	switch mode {
	case "", WriteModeAsync, WriteModeSync:
		return nil
	default:
		return fmt.Errorf("unrecognized write_mode %q: valid values are %q and %q", mode, WriteModeAsync, WriteModeSync)
	}
}

// validateLogLevel rejects an unrecognized log_level. An empty value is allowed
// and means no client logging is attached.
func validateLogLevel(level string) error {
	switch level {
	case "", "debug", "info", "error":
		return nil
	default:
		return fmt.Errorf("unrecognized log_level %q: valid values are debug, info, error", level)
	}
}

// rejectUnwiredKey builds the validation error for a config key that the current
// Elasticsearch client no longer reads, pointing operators at the PR that explains
// the change. The migration is always the same: remove the key.
func rejectUnwiredKey(key, reason string) error {
	return fmt.Errorf(
		"'%s' is no longer supported: %s; please remove the setting "+
			"(see https://github.com/jaegertracing/jaeger/pull/9076)",
		key, reason,
	)
}
