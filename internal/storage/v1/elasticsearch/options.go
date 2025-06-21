// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"flag"
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/internal/bearertoken"
	"github.com/jaegertracing/jaeger/internal/config/tlscfg"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

const (
	suffixUsername                       = ".username"
	suffixPassword                       = ".password"
	suffixSniffer                        = ".sniffer"
	suffixDisableHealthCheck             = ".disable-health-check"
	suffixSnifferTLSEnabled              = ".sniffer-tls-enabled"
	suffixTokenPath                      = ".token-file"
	suffixPasswordPath                   = ".password-file"
	suffixServerURLs                     = ".server-urls"
	suffixRemoteReadClusters             = ".remote-read-clusters"
	suffixMaxSpanAge                     = ".max-span-age"
	suffixAdaptiveSamplingLookback       = ".adaptive-sampling.lookback"
	suffixNumShards                      = ".num-shards"
	suffixNumReplicas                    = ".num-replicas"
	suffixPrioritySpanTemplate           = ".prioirity-span-template"
	suffixPriorityServiceTemplate        = ".prioirity-service-template"
	suffixPriorityDependenciesTemplate   = ".prioirity-dependencies-template"
	suffixBulkSize                       = ".bulk.size"
	suffixBulkWorkers                    = ".bulk.workers"
	suffixBulkActions                    = ".bulk.actions"
	suffixBulkFlushInterval              = ".bulk.flush-interval"
	suffixTimeout                        = ".timeout"
	suffixIndexPrefix                    = ".index-prefix"
	suffixIndexDateSeparator             = ".index-date-separator"
	suffixIndexRolloverFrequencySpans    = ".index-rollover-frequency-spans"
	suffixIndexRolloverFrequencyServices = ".index-rollover-frequency-services"
	suffixIndexRolloverFrequencySampling = ".index-rollover-frequency-adaptive-sampling"
	suffixServiceCacheTTL                = ".service-cache-ttl"
	suffixTagsAsFields                   = ".tags-as-fields"
	suffixTagsAsFieldsAll                = suffixTagsAsFields + ".all"
	suffixTagsAsFieldsInclude            = suffixTagsAsFields + ".include"
	suffixTagsFile                       = suffixTagsAsFields + ".config-file"
	suffixTagDeDotChar                   = suffixTagsAsFields + ".dot-replacement"
	suffixReadAlias                      = ".use-aliases"
	suffixUseILM                         = ".use-ilm"
	suffixCreateIndexTemplate            = ".create-index-templates"
	suffixEnabled                        = ".enabled"
	suffixVersion                        = ".version"
	suffixMaxDocCount                    = ".max-doc-count"
	suffixLogLevel                       = ".log-level"
	suffixSendGetBodyAs                  = ".send-get-body-as"
	suffixHTTPCompression                = ".http-compression"
	// default number of documents to return from a query (elasticsearch allowed limit)
	// see search.max_buckets and index.max_result_window
	defaultMaxDocCount        = 10_000
	defaultServerURL          = "http://127.0.0.1:9200"
	defaultRemoteReadClusters = ""
	// default separator for Elasticsearch index date layout.
	defaultIndexDateSeparator = "-"

	defaultIndexRolloverFrequency = "day"
	defaultSendGetBodyAs          = ""
	defaultIndexPrefix            = ""
)

var defaultIndexOptions = config.IndexOptions{
	DateLayout:        initDateLayout(defaultIndexRolloverFrequency, defaultIndexDateSeparator),
	RolloverFrequency: defaultIndexRolloverFrequency,
	Shards:            5,
	Replicas:          ptr(int64(1)),
	Priority:          0,
}

// TODO this should be moved next to config.Configuration struct (maybe ./flags package)

// Options contains various type of Elasticsearch configs and provides the ability
// to bind them to command line flag and apply overlays, so that some configurations
// (e.g. archive) may be underspecified and infer the rest of its parameters from primary.
type Options struct {
	// TODO: remove indirection
	Config namespaceConfig `mapstructure:",squash"`
}

type namespaceConfig struct {
	config.Configuration `mapstructure:",squash"`
	namespace            string
}

// NewOptions creates a new Options struct.
func NewOptions(namespace string) *Options {
	// TODO all default values should be defined via cobra flags
	defaultConfig := DefaultConfig()
	options := &Options{
		Config: namespaceConfig{
			Configuration: defaultConfig,
			namespace:     namespace,
		},
	}

	return options
}

func (cfg *namespaceConfig) getTLSFlagsConfig() tlscfg.ClientFlagsConfig {
	return tlscfg.ClientFlagsConfig{
		Prefix: cfg.namespace,
	}
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}

// safeDerefInt64 safely dereferences a *int64 for use in flagSet.Int64.
// If the pointer is nil (meaning no config was set), returns 0 as neutral default.
func safeDerefInt64(ptr *int64) int64 {
	if ptr != nil {
		return *ptr
	}
	return 0
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	addFlags(flagSet, &opt.Config)
}

func addFlags(flagSet *flag.FlagSet, nsConfig *namespaceConfig) {
	flagSet.String(
		nsConfig.namespace+suffixUsername,
		nsConfig.Configuration.Authentication.BasicAuthentication.Username,
		"The username required by Elasticsearch. The basic authentication also loads CA if it is specified.")
	flagSet.String(
		nsConfig.namespace+suffixPassword,
		nsConfig.Configuration.Authentication.BasicAuthentication.Password,
		"The password required by Elasticsearch")
	flagSet.String(
		nsConfig.namespace+suffixTokenPath,
		nsConfig.Configuration.Authentication.BearerTokenAuthentication.FilePath,
		"Path to a file containing bearer token. This flag also loads CA if it is specified.")
	flagSet.String(
		nsConfig.namespace+suffixPasswordPath,
		nsConfig.Configuration.Authentication.BasicAuthentication.PasswordFilePath,
		"Path to a file containing password. This file is watched for changes.")
	flagSet.Bool(
		nsConfig.namespace+suffixSniffer,
		nsConfig.Configuration.Sniffing.Enabled,
		"The sniffer config for Elasticsearch; client uses sniffing process to find all nodes automatically, disable if not required")
	flagSet.Bool(
		nsConfig.namespace+suffixDisableHealthCheck,
		nsConfig.Configuration.DisableHealthCheck,
		"Disable the Elasticsearch health check.")
	flagSet.String(
		nsConfig.namespace+suffixServerURLs,
		defaultServerURL,
		"The comma-separated list of Elasticsearch servers, must be full url i.e. http://localhost:9200")
	flagSet.String(
		nsConfig.namespace+suffixRemoteReadClusters,
		defaultRemoteReadClusters,
		"Comma-separated list of Elasticsearch remote cluster names for cross-cluster querying."+
			"See Elasticsearch remote clusters and cross-cluster query api.")
	flagSet.Duration(
		nsConfig.namespace+suffixTimeout,
		nsConfig.Configuration.QueryTimeout,
		"Timeout used for queries. A Timeout of zero means no timeout")
	flagSet.Int64(
		nsConfig.namespace+suffixNumShards,
		nsConfig.Configuration.Indices.Spans.Shards,
		"The number of shards per index in Elasticsearch")
	flagSet.Duration(
		nsConfig.namespace+suffixServiceCacheTTL,
		nsConfig.Configuration.ServiceCacheTTL,
		"The TTL for the cache of known service names")
	flagSet.Int64(
		nsConfig.namespace+suffixNumReplicas,
		safeDerefInt64(nsConfig.Configuration.Indices.Spans.Replicas),
		"The number of replicas per index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffixPrioritySpanTemplate,
		nsConfig.Configuration.Indices.Spans.Priority,
		"Priority of jaeger-span index template (ESv8 only)")
	flagSet.Int64(
		nsConfig.namespace+suffixPriorityServiceTemplate,
		nsConfig.Configuration.Indices.Services.Priority,
		"Priority of jaeger-service index template (ESv8 only)")
	flagSet.Int64(
		nsConfig.namespace+suffixPriorityDependenciesTemplate,
		nsConfig.Configuration.Indices.Dependencies.Priority,
		"Priority of jaeger-dependecies index template (ESv8 only)")
	flagSet.Int(
		nsConfig.namespace+suffixBulkSize,
		nsConfig.Configuration.BulkProcessing.MaxBytes,
		"The number of bytes that the bulk requests can take up before the bulk processor decides to commit")
	flagSet.Int(
		nsConfig.namespace+suffixBulkWorkers,
		nsConfig.Configuration.BulkProcessing.Workers,
		"The number of workers that are able to receive bulk requests and eventually commit them to Elasticsearch")
	flagSet.Int(
		nsConfig.namespace+suffixBulkActions,
		nsConfig.Configuration.BulkProcessing.MaxActions,
		"The number of requests that can be enqueued before the bulk processor decides to commit")
	flagSet.Duration(
		nsConfig.namespace+suffixBulkFlushInterval,
		nsConfig.Configuration.BulkProcessing.FlushInterval,
		"A time.Duration after which bulk requests are committed, regardless of other thresholds. Set to zero to disable. By default, this is disabled.")
	flagSet.String(
		nsConfig.namespace+suffixIndexPrefix,
		string(nsConfig.Configuration.Indices.IndexPrefix),
		"Optional prefix of Jaeger indices. For example \"production\" creates \"production-jaeger-*\".")
	flagSet.String(
		nsConfig.namespace+suffixIndexDateSeparator,
		defaultIndexDateSeparator,
		"Optional date separator of Jaeger indices. For example \".\" creates \"jaeger-span-2020.11.20\".")
	flagSet.String(
		nsConfig.namespace+suffixIndexRolloverFrequencySpans,
		nsConfig.Configuration.Indices.Spans.RolloverFrequency,
		"Rotates jaeger-span indices over the given period. For example \"day\" creates \"jaeger-span-yyyy-MM-dd\" every day after UTC 12AM. Valid options: [hour, day]. "+
			"This does not delete old indices. For details on complete index management solutions supported by Jaeger, refer to: https://www.jaegertracing.io/docs/deployment/#elasticsearch-rollover")
	flagSet.String(
		nsConfig.namespace+suffixIndexRolloverFrequencyServices,
		nsConfig.Configuration.Indices.Services.RolloverFrequency,
		"Rotates jaeger-service indices over the given period. For example \"day\" creates \"jaeger-service-yyyy-MM-dd\" every day after UTC 12AM. Valid options: [hour, day]. "+
			"This does not delete old indices. For details on complete index management solutions supported by Jaeger, refer to: https://www.jaegertracing.io/docs/deployment/#elasticsearch-rollover")
	flagSet.String(
		nsConfig.namespace+suffixIndexRolloverFrequencySampling,
		nsConfig.Configuration.Indices.Sampling.RolloverFrequency,
		"Rotates jaeger-sampling indices over the given period. For example \"day\" creates \"jaeger-sampling-yyyy-MM-dd\" every day after UTC 12AM. Valid options: [hour, day]. "+
			"This does not delete old indices. For details on complete index management solutions supported by Jaeger, refer to: https://www.jaegertracing.io/docs/deployment/#elasticsearch-rollover")
	flagSet.Bool(
		nsConfig.namespace+suffixTagsAsFieldsAll,
		nsConfig.Configuration.Tags.AllAsFields,
		"(experimental) Store all span and process tags as object fields. If true "+suffixTagsFile+" and "+suffixTagsAsFieldsInclude+" is ignored. Binary tags are always stored as nested objects.")
	flagSet.String(
		nsConfig.namespace+suffixTagsAsFieldsInclude,
		nsConfig.Configuration.Tags.Include,
		"(experimental) Comma delimited list of tag keys which will be stored as object fields. Merged with the contents of "+suffixTagsFile)
	flagSet.String(
		nsConfig.namespace+suffixTagsFile,
		nsConfig.Configuration.Tags.File,
		"(experimental) Optional path to a file containing tag keys which will be stored as object fields. Each key should be on a separate line.  Merged with "+suffixTagsAsFieldsInclude)
	flagSet.String(
		nsConfig.namespace+suffixTagDeDotChar,
		nsConfig.Configuration.Tags.DotReplacement,
		"(experimental) The character used to replace dots (\".\") in tag keys stored as object fields.")
	flagSet.Bool(
		nsConfig.namespace+suffixReadAlias,
		nsConfig.Configuration.UseReadWriteAliases,
		"Use read and write aliases for indices. Use this option with Elasticsearch rollover "+
			"API. It requires an external component to create aliases before startup and then performing its management. "+
			"Note that es"+suffixMaxSpanAge+" will influence trace search window start times.")
	flagSet.Bool(
		nsConfig.namespace+suffixUseILM,
		nsConfig.Configuration.UseILM,
		"(experimental) Option to enable ILM for jaeger span & service indices. Use this option with  "+nsConfig.namespace+suffixReadAlias+". "+
			"It requires an external component to create aliases before startup and then performing its management. "+
			"ILM policy must be manually created in ES before startup. Supported only for elasticsearch version 7+.")
	flagSet.Bool(
		nsConfig.namespace+suffixCreateIndexTemplate,
		nsConfig.Configuration.CreateIndexTemplates,
		"Create index templates at application startup. Set to false when templates are installed manually.")
	flagSet.Uint(
		nsConfig.namespace+suffixVersion,
		0,
		"The major Elasticsearch version. If not specified, the value will be auto-detected from Elasticsearch.")
	flagSet.Bool(
		nsConfig.namespace+suffixSnifferTLSEnabled,
		nsConfig.Configuration.Sniffing.UseHTTPS,
		"Option to enable TLS when sniffing an Elasticsearch Cluster ; client uses sniffing process to find all nodes automatically, disabled by default")
	flagSet.Int(
		nsConfig.namespace+suffixMaxDocCount,
		nsConfig.Configuration.MaxDocCount,
		"The maximum document count to return from an Elasticsearch query. This will also apply to aggregations.")
	flagSet.String(
		nsConfig.namespace+suffixLogLevel,
		nsConfig.Configuration.LogLevel,
		"The Elasticsearch client log-level. Valid levels: [debug, info, error]")
	flagSet.String(
		nsConfig.namespace+suffixSendGetBodyAs,
		nsConfig.Configuration.SendGetBodyAs,
		"HTTP verb for requests that contain a body [GET, POST].")
	flagSet.Bool(
		nsConfig.namespace+suffixHTTPCompression,
		nsConfig.Configuration.HTTPCompression,
		"Use gzip compression for requests to ElasticSearch.")
	flagSet.Duration(
		nsConfig.namespace+suffixAdaptiveSamplingLookback,
		nsConfig.Configuration.AdaptiveSamplingLookback,
		"How far back to look for the latest adaptive sampling probabilities")
	if nsConfig.namespace == archiveNamespace {
		flagSet.Bool(
			nsConfig.namespace+suffixEnabled,
			false,
			"Enable extra storage")
	} else {
		// MaxSpanAge is only relevant when searching for unarchived traces.
		// Archived traces are searched with no look-back limit.
		flagSet.Duration(
			nsConfig.namespace+suffixMaxSpanAge,
			nsConfig.Configuration.MaxSpanAge,
			"The maximum lookback for spans in Elasticsearch")
	}
	nsConfig.getTLSFlagsConfig().AddFlags(flagSet)
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	initFromViper(&opt.Config, v)
}

func initFromViper(cfg *namespaceConfig, v *viper.Viper) {
	cfg.Configuration.Authentication.BasicAuthentication.Username = v.GetString(cfg.namespace + suffixUsername)
	cfg.Configuration.Authentication.BasicAuthentication.Password = v.GetString(cfg.namespace + suffixPassword)
	cfg.Configuration.Authentication.BearerTokenAuthentication.FilePath = v.GetString(cfg.namespace + suffixTokenPath)
	cfg.Configuration.Authentication.BasicAuthentication.PasswordFilePath = v.GetString(cfg.namespace + suffixPasswordPath)
	cfg.Configuration.Sniffing.Enabled = v.GetBool(cfg.namespace + suffixSniffer)
	cfg.Configuration.Sniffing.UseHTTPS = v.GetBool(cfg.namespace + suffixSnifferTLSEnabled)
	cfg.Configuration.DisableHealthCheck = v.GetBool(cfg.namespace + suffixDisableHealthCheck)
	cfg.Configuration.Servers = strings.Split(stripWhiteSpace(v.GetString(cfg.namespace+suffixServerURLs)), ",")
	cfg.Configuration.MaxSpanAge = v.GetDuration(cfg.namespace + suffixMaxSpanAge)
	cfg.Configuration.AdaptiveSamplingLookback = v.GetDuration(cfg.namespace + suffixAdaptiveSamplingLookback)

	cfg.Configuration.Indices.Spans.Shards = v.GetInt64(cfg.namespace + suffixNumShards)
	cfg.Configuration.Indices.Services.Shards = v.GetInt64(cfg.namespace + suffixNumShards)
	cfg.Configuration.Indices.Sampling.Shards = v.GetInt64(cfg.namespace + suffixNumShards)
	cfg.Configuration.Indices.Dependencies.Shards = v.GetInt64(cfg.namespace + suffixNumShards)

	// Note: We use a pointer type for Replicas to distinguish between "unset" and "explicit 0".
	// Each field receives its own pointer to avoid accidental shared state.
	replicas := v.GetInt64(cfg.namespace + suffixNumReplicas)
	cfg.Configuration.Indices.Spans.Replicas = ptr(replicas)
	cfg.Configuration.Indices.Services.Replicas = ptr(replicas)
	cfg.Configuration.Indices.Sampling.Replicas = ptr(replicas)
	cfg.Configuration.Indices.Dependencies.Replicas = ptr(replicas)

	cfg.Configuration.Indices.Spans.Priority = v.GetInt64(cfg.namespace + suffixPrioritySpanTemplate)
	cfg.Configuration.Indices.Services.Priority = v.GetInt64(cfg.namespace + suffixPriorityServiceTemplate)
	// cfg.Configuration.Indices.Sampling does not have a separate flag
	cfg.Configuration.Indices.Dependencies.Priority = v.GetInt64(cfg.namespace + suffixPriorityDependenciesTemplate)

	cfg.Configuration.BulkProcessing.MaxBytes = v.GetInt(cfg.namespace + suffixBulkSize)
	cfg.Configuration.BulkProcessing.Workers = v.GetInt(cfg.namespace + suffixBulkWorkers)
	cfg.Configuration.BulkProcessing.MaxActions = v.GetInt(cfg.namespace + suffixBulkActions)
	cfg.Configuration.BulkProcessing.FlushInterval = v.GetDuration(cfg.namespace + suffixBulkFlushInterval)
	cfg.Configuration.QueryTimeout = v.GetDuration(cfg.namespace + suffixTimeout)
	cfg.Configuration.ServiceCacheTTL = v.GetDuration(cfg.namespace + suffixServiceCacheTTL)
	indexPrefix := v.GetString(cfg.namespace + suffixIndexPrefix)

	cfg.Configuration.Indices.IndexPrefix = config.IndexPrefix(indexPrefix)

	cfg.Configuration.Tags.AllAsFields = v.GetBool(cfg.namespace + suffixTagsAsFieldsAll)
	cfg.Configuration.Tags.Include = v.GetString(cfg.namespace + suffixTagsAsFieldsInclude)
	cfg.Configuration.Tags.File = v.GetString(cfg.namespace + suffixTagsFile)
	cfg.Configuration.Tags.DotReplacement = v.GetString(cfg.namespace + suffixTagDeDotChar)
	cfg.Configuration.UseReadWriteAliases = v.GetBool(cfg.namespace + suffixReadAlias)
	cfg.Configuration.Enabled = v.GetBool(cfg.namespace + suffixEnabled)
	cfg.Configuration.CreateIndexTemplates = v.GetBool(cfg.namespace + suffixCreateIndexTemplate)
	cfg.Configuration.Version = v.GetUint(cfg.namespace + suffixVersion)
	cfg.Configuration.LogLevel = v.GetString(cfg.namespace + suffixLogLevel)
	cfg.Configuration.SendGetBodyAs = v.GetString(cfg.namespace + suffixSendGetBodyAs)
	cfg.Configuration.HTTPCompression = v.GetBool(cfg.namespace + suffixHTTPCompression)

	cfg.Configuration.MaxDocCount = v.GetInt(cfg.namespace + suffixMaxDocCount)
	cfg.Configuration.UseILM = v.GetBool(cfg.namespace + suffixUseILM)

	// TODO: Need to figure out a better way for do this.
	cfg.Configuration.Authentication.BearerTokenAuthentication.AllowFromContext = v.GetBool(bearertoken.StoragePropagationKey)

	remoteReadClusters := stripWhiteSpace(v.GetString(cfg.namespace + suffixRemoteReadClusters))
	if len(remoteReadClusters) > 0 {
		cfg.Configuration.RemoteReadClusters = strings.Split(remoteReadClusters, ",")
	}

	cfg.Configuration.Indices.Spans.RolloverFrequency = strings.ToLower(v.GetString(cfg.namespace + suffixIndexRolloverFrequencySpans))
	cfg.Configuration.Indices.Services.RolloverFrequency = strings.ToLower(v.GetString(cfg.namespace + suffixIndexRolloverFrequencyServices))
	cfg.Configuration.Indices.Sampling.RolloverFrequency = strings.ToLower(v.GetString(cfg.namespace + suffixIndexRolloverFrequencySampling))

	separator := v.GetString(cfg.namespace + suffixIndexDateSeparator)
	cfg.Configuration.Indices.Spans.DateLayout = initDateLayout(cfg.Configuration.Indices.Spans.RolloverFrequency, separator)
	cfg.Configuration.Indices.Services.DateLayout = initDateLayout(cfg.Configuration.Indices.Services.RolloverFrequency, separator)
	cfg.Configuration.Indices.Sampling.DateLayout = initDateLayout(cfg.Configuration.Indices.Sampling.RolloverFrequency, separator)

	// Daily is recommended for dependencies calculation, and this index size is very small
	cfg.Configuration.Indices.Dependencies.DateLayout = initDateLayout(cfg.Configuration.Indices.Dependencies.DateLayout, separator)
	tlsconfig, err := cfg.getTLSFlagsConfig().InitFromViper(v)
	if err != nil {
		// TODO refactor to be able to return error
		log.Fatal(err)
	}
	cfg.Configuration.TLS = tlsconfig
}

// GetPrimary returns primary configuration.
func (opt *Options) GetConfig() *config.Configuration {
	return &opt.Config.Configuration
}

// stripWhiteSpace removes all whitespace characters from a string
func stripWhiteSpace(str string) string {
	return strings.ReplaceAll(str, " ", "")
}

func initDateLayout(rolloverFreq, sep string) string {
	// default to daily format
	indexLayout := "2006" + sep + "01" + sep + "02"
	if rolloverFreq == "hour" {
		indexLayout = indexLayout + sep + "15"
	}
	return indexLayout
}

func DefaultConfig() config.Configuration {
	return config.Configuration{
		Authentication: config.Authentication{
			BasicAuthentication: config.BasicAuthentication{
				Username: "",
				Password: "",
			},
		},
		Sniffing: config.Sniffing{
			Enabled: false,
		},
		DisableHealthCheck:       false,
		MaxSpanAge:               72 * time.Hour,
		AdaptiveSamplingLookback: 72 * time.Hour,
		BulkProcessing: config.BulkProcessing{
			MaxBytes:      5 * 1000 * 1000,
			Workers:       1,
			MaxActions:    1000,
			FlushInterval: time.Millisecond * 200,
		},
		Tags: config.TagsAsFields{
			DotReplacement: "@",
		},
		Enabled:              true,
		CreateIndexTemplates: true,
		Version:              0,
		UseReadWriteAliases:  false,
		UseILM:               false,
		Servers:              []string{defaultServerURL},
		RemoteReadClusters:   []string{},
		MaxDocCount:          defaultMaxDocCount,
		LogLevel:             "error",
		SendGetBodyAs:        defaultSendGetBodyAs,
		HTTPCompression:      true,
		Indices: config.Indices{
			Spans:        defaultIndexOptions,
			Services:     defaultIndexOptions,
			Dependencies: defaultIndexOptions,
			Sampling:     defaultIndexOptions,
		},
	}
}
