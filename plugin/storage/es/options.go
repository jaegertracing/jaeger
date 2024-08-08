// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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

package es

import (
	"flag"
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/es/config"
)

const (
	suffix                         = "."
	suffixUsername                 = ".username"
	suffixPassword                 = ".password"
	suffixSniffer                  = ".sniffer"
	suffixSnifferTLSEnabled        = ".sniffer-tls-enabled"
	suffixTokenPath                = ".token-file"
	suffixPasswordPath             = ".password-file"
	suffixServerURLs               = ".server-urls"
	suffixRemoteReadClusters       = ".remote-read-clusters"
	suffixMaxSpanAge               = ".max-span-age"
	suffixAdaptiveSamplingLookback = ".adaptive-sampling.lookback"
	suffixNumShards                = ".num-shards"
	suffixNumReplicas              = ".num-replicas"
	suffixBulkSize                 = ".bulk.size"
	suffixBulkWorkers              = ".bulk.workers"
	suffixBulkActions              = ".bulk.actions"
	suffixBulkFlushInterval        = ".bulk.flush-interval"
	suffixTimeout                  = ".timeout"
	suffixIndexPrefix              = ".index-prefix"
	suffixIndexDateSeparator       = ".index-date-separator"
	suffixServiceCacheTTL          = ".service-cache-ttl"
	suffixTagsAsFields             = ".tags-as-fields"
	suffixTagsAsFieldsAll          = suffixTagsAsFields + ".all"
	suffixTagsAsFieldsInclude      = suffixTagsAsFields + ".include"
	suffixTagsFile                 = suffixTagsAsFields + ".config-file"
	suffixTagDeDotChar             = suffixTagsAsFields + ".dot-replacement"
	suffixReadAlias                = ".use-aliases"
	suffixUseILM                   = ".use-ilm"
	suffixCreateIndexTemplate      = ".create-index-templates"
	suffixEnabled                  = ".enabled"
	suffixVersion                  = ".version"
	suffixMaxDocCount              = ".max-doc-count"
	suffixLogLevel                 = ".log-level"
	suffixSendGetBodyAs            = ".send-get-body-as"
	// default number of documents to return from a query (elasticsearch allowed limit)
	// see search.max_buckets and index.max_result_window
	defaultMaxDocCount        = 10_000
	defaultServerURL          = "http://127.0.0.1:9200"
	defaultRemoteReadClusters = ""
	// default separator for Elasticsearch index date layout.
	defaultIndexDateSeparator     = "-"
	defaultIndexRolloverFrequency = "day"
	defaultSendGetBodyAs          = ""
)

var defaultIndexOptions = config.IndexOptions{
	DateLayout:          initDateLayout(defaultIndexRolloverFrequency, defaultIndexDateSeparator),
	RolloverFrequency:   defaultIndexRolloverFrequency,
	TemplateNumShards:   5,
	TemplateNumReplicas: 1,
	TemplatePriority:    0,
}

// TODO this should be moved next to config.Configuration struct (maybe ./flags package)

// Options contains various type of Elasticsearch configs and provides the ability
// to bind them to command line flag and apply overlays, so that some configurations
// (e.g. archive) may be underspecified and infer the rest of its parameters from primary.
type Options struct {
	Primary namespaceConfig `mapstructure:",squash"`

	others map[string]*namespaceConfig
}

type namespaceConfig struct {
	config.Configuration `mapstructure:",squash"`
	namespace            string
}

// NewOptions creates a new Options struct.
func NewOptions(primaryNamespace string, otherNamespaces ...string) *Options {
	// TODO all default values should be defined via cobra flags
	defaultConfig := DefaultConfig()
	options := &Options{
		Primary: namespaceConfig{
			Configuration: defaultConfig,
			namespace:     primaryNamespace,
		},
		others: make(map[string]*namespaceConfig, len(otherNamespaces)),
	}

	// Other namespaces need to be explicitly enabled.
	defaultConfig.Enabled = false
	for _, namespace := range otherNamespaces {
		options.others[namespace] = &namespaceConfig{
			Configuration: defaultConfig,
			namespace:     namespace,
		}
	}

	return options
}

func (config *namespaceConfig) getTLSFlagsConfig() tlscfg.ClientFlagsConfig {
	return tlscfg.ClientFlagsConfig{
		Prefix: config.namespace,
	}
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	addFlags(flagSet, &opt.Primary)
	for _, cfg := range opt.others {
		addFlags(flagSet, cfg)
	}
}

func addFlags(flagSet *flag.FlagSet, nsConfig *namespaceConfig) {
	flagSet.String(
		nsConfig.namespace+suffixUsername,
		nsConfig.Username,
		"The username required by Elasticsearch. The basic authentication also loads CA if it is specified.")
	flagSet.String(
		nsConfig.namespace+suffixPassword,
		nsConfig.Password,
		"The password required by Elasticsearch")
	flagSet.String(
		nsConfig.namespace+suffixTokenPath,
		nsConfig.TokenFilePath,
		"Path to a file containing bearer token. This flag also loads CA if it is specified.")
	flagSet.String(
		nsConfig.namespace+suffixPasswordPath,
		nsConfig.PasswordFilePath,
		"Path to a file containing password. This file is watched for changes.")
	flagSet.Bool(
		nsConfig.namespace+suffixSniffer,
		nsConfig.Sniffer,
		"The sniffer config for Elasticsearch; client uses sniffing process to find all nodes automatically, disable if not required")
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
		nsConfig.Timeout,
		"Timeout used for queries. A Timeout of zero means no timeout")
	flagSet.Duration(
		nsConfig.namespace+suffixServiceCacheTTL,
		nsConfig.ServiceCacheTTL,
		"The TTL for the cache of known service names",
	)
	flagSet.Int64(
		nsConfig.namespace+suffixNumShards,
		defaultIndexOptions.TemplateNumShards,
		"(deprecated, will be replaced with num-shards-span,num-shards-service,num-shards-sampling,num-shards-dependencies, if .num-shards set, it will be used as global settings for span,service,sampling,dependencies index) The number of shards per index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffixNumReplicas,
		defaultIndexOptions.TemplateNumReplicas,
		"(deprecated, will be replaced with num-replicas-span,num-replicas-service,num-replicas-sampling,num-replicas-dependencies, if .num-replicas set, it will be used as global settings for span,service,sampling,dependencies index) The number of replicas per index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffix+config.NumShardSpanFlag(),
		nsConfig.Indices.Spans.TemplateNumShards,
		"The number of shards per span index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffix+config.NumReplicaSpanFlag(),
		nsConfig.Indices.Spans.TemplateNumReplicas,
		"The number of replicas per span index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffix+config.NumShardServiceFlag(),
		nsConfig.Indices.Services.TemplateNumShards,
		"The number of shards per service index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffix+config.NumReplicaServiceFlag(),
		nsConfig.Indices.Services.TemplateNumReplicas,
		"The number of replicas per service index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffix+config.NumShardSamplingFlag(),
		nsConfig.Indices.Sampling.TemplateNumShards,
		"The number of shards per sampling index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffix+config.NumReplicaSamplingFlag(),
		nsConfig.Indices.Sampling.TemplateNumReplicas,
		"The number of replicas per sampling index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffix+config.NumShardDependenciesFlag(),
		nsConfig.Indices.Dependencies.TemplateNumShards,
		"The number of shards per dependencies index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffix+config.NumReplicaDependenciesFlag(),
		nsConfig.Indices.Dependencies.TemplateNumReplicas,
		"The number of replicas per dependencies index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffix+config.PrioritySpanTemplateFlag(),
		nsConfig.Indices.Spans.TemplatePriority,
		"Priority of jaeger-span index template (ESv8 only)")
	flagSet.Int64(
		nsConfig.namespace+suffix+config.PriorityServiceTemplateFlag(),
		nsConfig.Indices.Services.TemplatePriority,
		"Priority of jaeger-service index template (ESv8 only)")
	flagSet.Int64(
		nsConfig.namespace+suffix+config.PriorityDependenciesTemplateFlag(),
		nsConfig.Indices.Dependencies.TemplatePriority,
		"Priority of jaeger-dependecies index template (ESv8 only)")
	flagSet.Int(
		nsConfig.namespace+suffixBulkSize,
		nsConfig.BulkSize,
		"The number of bytes that the bulk requests can take up before the bulk processor decides to commit")
	flagSet.Int(
		nsConfig.namespace+suffixBulkWorkers,
		nsConfig.BulkWorkers,
		"The number of workers that are able to receive bulk requests and eventually commit them to Elasticsearch")
	flagSet.Int(
		nsConfig.namespace+suffixBulkActions,
		nsConfig.BulkActions,
		"The number of requests that can be enqueued before the bulk processor decides to commit")
	flagSet.Duration(
		nsConfig.namespace+suffixBulkFlushInterval,
		nsConfig.BulkFlushInterval,
		"A time.Duration after which bulk requests are committed, regardless of other thresholds. Set to zero to disable. By default, this is disabled.")
	flagSet.String(
		nsConfig.namespace+suffixIndexPrefix,
		nsConfig.IndexPrefix,
		"Optional prefix of Jaeger indices. For example \"production\" creates \"production-jaeger-*\".")
	flagSet.String(
		nsConfig.namespace+suffixIndexDateSeparator,
		defaultIndexDateSeparator,
		"Optional date separator of Jaeger indices. For example \".\" creates \"jaeger-span-2020.11.20\".")
	flagSet.String(
		nsConfig.namespace+suffix+config.IndexRolloverFrequencySpanFlag(),
		nsConfig.Indices.Spans.RolloverFrequency,
		"Rotates jaeger-span indices over the given period. For example \"day\" creates \"jaeger-span-yyyy-MM-dd\" every day after UTC 12AM. Valid options: [hour, day]. "+
			"This does not delete old indices. For details on complete index management solutions supported by Jaeger, refer to: https://www.jaegertracing.io/docs/deployment/#elasticsearch-rollover")
	flagSet.String(
		nsConfig.namespace+suffix+config.IndexRolloverFrequencyServiceFlag(),
		nsConfig.Indices.Services.RolloverFrequency,
		"Rotates jaeger-service indices over the given period. For example \"day\" creates \"jaeger-service-yyyy-MM-dd\" every day after UTC 12AM. Valid options: [hour, day]. "+
			"This does not delete old indices. For details on complete index management solutions supported by Jaeger, refer to: https://www.jaegertracing.io/docs/deployment/#elasticsearch-rollover")
	flagSet.String(
		nsConfig.namespace+suffix+config.IndexRolloverFrequencySamplingFlag(),
		nsConfig.Indices.Sampling.RolloverFrequency,
		"Rotates jaeger-sampling indices over the given period. For example \"day\" creates \"jaeger-sampling-yyyy-MM-dd\" every day after UTC 12AM. Valid options: [hour, day]. "+
			"This does not delete old indices. For details on complete index management solutions supported by Jaeger, refer to: https://www.jaegertracing.io/docs/deployment/#elasticsearch-rollover")
	flagSet.String(
		nsConfig.namespace+suffix+config.IndexRolloverFrequencyDependenciesFlag(),
		nsConfig.Indices.Dependencies.RolloverFrequency,
		"Rotates jaeger-dependencies indices over the given period. For example \"day\" creates \"jaeger-dependencies-yyyy-MM-dd\" every day after UTC 12AM. Valid options: [hour, day]. "+
			"This does not delete old indices. For details on complete index management solutions supported by Jaeger, refer to: https://www.jaegertracing.io/docs/deployment/#elasticsearch-rollover")
	flagSet.Bool(
		nsConfig.namespace+suffixTagsAsFieldsAll,
		nsConfig.Tags.AllAsFields,
		"(experimental) Store all span and process tags as object fields. If true "+suffixTagsFile+" and "+suffixTagsAsFieldsInclude+" is ignored. Binary tags are always stored as nested objects.")
	flagSet.String(
		nsConfig.namespace+suffixTagsAsFieldsInclude,
		nsConfig.Tags.Include,
		"(experimental) Comma delimited list of tag keys which will be stored as object fields. Merged with the contents of "+suffixTagsFile)
	flagSet.String(
		nsConfig.namespace+suffixTagsFile,
		nsConfig.Tags.File,
		"(experimental) Optional path to a file containing tag keys which will be stored as object fields. Each key should be on a separate line.  Merged with "+suffixTagsAsFieldsInclude)
	flagSet.String(
		nsConfig.namespace+suffixTagDeDotChar,
		nsConfig.Tags.DotReplacement,
		"(experimental) The character used to replace dots (\".\") in tag keys stored as object fields.")
	flagSet.Bool(
		nsConfig.namespace+suffixReadAlias,
		nsConfig.UseReadWriteAliases,
		"Use read and write aliases for indices. Use this option with Elasticsearch rollover "+
			"API. It requires an external component to create aliases before startup and then performing its management. "+
			"Note that es"+suffixMaxSpanAge+" will influence trace search window start times.")
	flagSet.Bool(
		nsConfig.namespace+suffixUseILM,
		nsConfig.UseILM,
		"(experimental) Option to enable ILM for jaeger span & service indices. Use this option with  "+nsConfig.namespace+suffixReadAlias+". "+
			"It requires an external component to create aliases before startup and then performing its management. "+
			"ILM policy must be manually created in ES before startup. Supported only for elasticsearch version 7+.")
	flagSet.Bool(
		nsConfig.namespace+suffixCreateIndexTemplate,
		nsConfig.CreateIndexTemplates,
		"Create index templates at application startup. Set to false when templates are installed manually.")
	flagSet.Uint(
		nsConfig.namespace+suffixVersion,
		0,
		"The major Elasticsearch version. If not specified, the value will be auto-detected from Elasticsearch.")
	flagSet.Bool(
		nsConfig.namespace+suffixSnifferTLSEnabled,
		nsConfig.SnifferTLSEnabled,
		"Option to enable TLS when sniffing an Elasticsearch Cluster ; client uses sniffing process to find all nodes automatically, disabled by default")
	flagSet.Int(
		nsConfig.namespace+suffixMaxDocCount,
		nsConfig.MaxDocCount,
		"The maximum document count to return from an Elasticsearch query. This will also apply to aggregations.")
	flagSet.String(
		nsConfig.namespace+suffixLogLevel,
		nsConfig.LogLevel,
		"The Elasticsearch client log-level. Valid levels: [debug, info, error]")
	flagSet.String(
		nsConfig.namespace+suffixSendGetBodyAs,
		nsConfig.SendGetBodyAs,
		"HTTP verb for requests that contain a body [GET, POST].")
	flagSet.Duration(
		nsConfig.namespace+suffixAdaptiveSamplingLookback,
		nsConfig.AdaptiveSamplingLookback,
		"How far back to look for the latest adaptive sampling probabilities")
	if nsConfig.namespace == archiveNamespace {
		flagSet.Bool(
			nsConfig.namespace+suffixEnabled,
			nsConfig.Enabled,
			"Enable extra storage")
	} else {
		// MaxSpanAge is only relevant when searching for unarchived traces.
		// Archived traces are searched with no look-back limit.
		flagSet.Duration(
			nsConfig.namespace+suffixMaxSpanAge,
			nsConfig.MaxSpanAge,
			"The maximum lookback for spans in Elasticsearch")
	}
	nsConfig.getTLSFlagsConfig().AddFlags(flagSet)
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	initFromViper(&opt.Primary, v)
	for _, cfg := range opt.others {
		initFromViper(cfg, v)
	}
}

func initFromViper(cfg *namespaceConfig, v *viper.Viper) {
	overrideIndexShardsNums := func(deprecatedNum, newNum int) int64 {
		if deprecatedNum > 0 {
			return int64(deprecatedNum)
		} else {
			return int64(newNum)
		}
	}

	cfg.Username = v.GetString(cfg.namespace + suffixUsername)
	cfg.Password = v.GetString(cfg.namespace + suffixPassword)
	cfg.TokenFilePath = v.GetString(cfg.namespace + suffixTokenPath)
	cfg.PasswordFilePath = v.GetString(cfg.namespace + suffixPasswordPath)
	cfg.Sniffer = v.GetBool(cfg.namespace + suffixSniffer)
	cfg.SnifferTLSEnabled = v.GetBool(cfg.namespace + suffixSnifferTLSEnabled)
	cfg.Servers = strings.Split(stripWhiteSpace(v.GetString(cfg.namespace+suffixServerURLs)), ",")
	cfg.MaxSpanAge = v.GetDuration(cfg.namespace + suffixMaxSpanAge)
	cfg.AdaptiveSamplingLookback = v.GetDuration(cfg.namespace + suffixAdaptiveSamplingLookback)

	deprecatedNumShards := v.GetInt(cfg.namespace + suffixNumShards)
	deprecatedReplicaShards := v.GetInt(cfg.namespace + suffixNumReplicas)

	if deprecatedReplicaShards > 0 || deprecatedNumShards > 0 {
		log.Printf("using deprecated %s or %s", suffixNumShards, suffixNumReplicas)
	}
	cfg.Indices.Spans.TemplateNumShards = overrideIndexShardsNums(deprecatedNumShards, v.GetInt(cfg.namespace+suffix+config.NumShardSpanFlag()))
	cfg.Indices.Services.TemplateNumShards = overrideIndexShardsNums(deprecatedNumShards, v.GetInt(cfg.namespace+suffix+config.NumShardServiceFlag()))
	cfg.Indices.Sampling.TemplateNumShards = overrideIndexShardsNums(deprecatedNumShards, v.GetInt(cfg.namespace+suffix+config.NumShardSamplingFlag()))
	cfg.Indices.Dependencies.TemplateNumShards = overrideIndexShardsNums(deprecatedNumShards, v.GetInt(cfg.namespace+suffix+config.NumShardDependenciesFlag()))

	cfg.Indices.Spans.TemplateNumReplicas = overrideIndexShardsNums(deprecatedReplicaShards, v.GetInt(cfg.namespace+suffix+config.NumReplicaSpanFlag()))
	cfg.Indices.Services.TemplateNumReplicas = overrideIndexShardsNums(deprecatedReplicaShards, v.GetInt(cfg.namespace+suffix+config.NumReplicaServiceFlag()))
	cfg.Indices.Sampling.TemplateNumReplicas = overrideIndexShardsNums(deprecatedReplicaShards, v.GetInt(cfg.namespace+suffix+config.NumReplicaSamplingFlag()))
	cfg.Indices.Dependencies.TemplateNumReplicas = overrideIndexShardsNums(deprecatedReplicaShards, v.GetInt(cfg.namespace+suffix+config.NumReplicaDependenciesFlag()))

	cfg.Indices.Spans.TemplatePriority = v.GetInt64(cfg.namespace + suffix + config.PrioritySpanTemplateFlag())
	cfg.Indices.Services.TemplatePriority = v.GetInt64(cfg.namespace + suffix + config.PriorityServiceTemplateFlag())
	cfg.Indices.Dependencies.TemplatePriority = v.GetInt64(cfg.namespace + suffix + config.PriorityDependenciesTemplateFlag())
	cfg.Indices.Sampling.TemplatePriority = v.GetInt64(cfg.namespace + suffix + config.PrioritySpanTemplateFlag())

	cfg.BulkSize = v.GetInt(cfg.namespace + suffixBulkSize)
	cfg.BulkWorkers = v.GetInt(cfg.namespace + suffixBulkWorkers)
	cfg.BulkActions = v.GetInt(cfg.namespace + suffixBulkActions)
	cfg.BulkFlushInterval = v.GetDuration(cfg.namespace + suffixBulkFlushInterval)
	cfg.Timeout = v.GetDuration(cfg.namespace + suffixTimeout)
	cfg.ServiceCacheTTL = v.GetDuration(cfg.namespace + suffixServiceCacheTTL)
	cfg.IndexPrefix = v.GetString(cfg.namespace + suffixIndexPrefix)
	cfg.Tags.AllAsFields = v.GetBool(cfg.namespace + suffixTagsAsFieldsAll)
	cfg.Tags.Include = v.GetString(cfg.namespace + suffixTagsAsFieldsInclude)
	cfg.Tags.File = v.GetString(cfg.namespace + suffixTagsFile)
	cfg.Tags.DotReplacement = v.GetString(cfg.namespace + suffixTagDeDotChar)
	cfg.UseReadWriteAliases = v.GetBool(cfg.namespace + suffixReadAlias)
	cfg.Enabled = v.GetBool(cfg.namespace + suffixEnabled)
	cfg.CreateIndexTemplates = v.GetBool(cfg.namespace + suffixCreateIndexTemplate)
	cfg.Version = uint(v.GetInt(cfg.namespace + suffixVersion))
	cfg.LogLevel = v.GetString(cfg.namespace + suffixLogLevel)
	cfg.SendGetBodyAs = v.GetString(cfg.namespace + suffixSendGetBodyAs)

	cfg.MaxDocCount = v.GetInt(cfg.namespace + suffixMaxDocCount)
	cfg.UseILM = v.GetBool(cfg.namespace + suffixUseILM)

	// TODO: Need to figure out a better way for do this.
	cfg.AllowTokenFromContext = v.GetBool(bearertoken.StoragePropagationKey)

	remoteReadClusters := stripWhiteSpace(v.GetString(cfg.namespace + suffixRemoteReadClusters))
	if len(remoteReadClusters) > 0 {
		cfg.RemoteReadClusters = strings.Split(remoteReadClusters, ",")
	}

	cfg.Indices.Spans.RolloverFrequency = v.GetString(cfg.namespace + suffix + config.IndexRolloverFrequencySpanFlag())
	cfg.Indices.Services.RolloverFrequency = v.GetString(cfg.namespace + suffix + config.IndexRolloverFrequencyServiceFlag())
	cfg.Indices.Dependencies.RolloverFrequency = v.GetString(cfg.namespace + suffix + config.IndexRolloverFrequencyDependenciesFlag())
	cfg.Indices.Sampling.RolloverFrequency = v.GetString(cfg.namespace + suffix + config.IndexRolloverFrequencySamplingFlag())

	separator := v.GetString(cfg.namespace + suffixIndexDateSeparator)
	cfg.Indices.Spans.DateLayout = initDateLayout(cfg.Indices.Spans.RolloverFrequency, separator)
	cfg.Indices.Services.DateLayout = initDateLayout(cfg.Indices.Services.RolloverFrequency, separator)
	cfg.Indices.Sampling.DateLayout = initDateLayout(cfg.Indices.Sampling.RolloverFrequency, separator)

	// Daily is recommended for dependencies calculation, and this index size is very small
	cfg.Indices.Dependencies.DateLayout = initDateLayout(cfg.Indices.Dependencies.RolloverFrequency, separator)
	var err error
	cfg.TLS, err = cfg.getTLSFlagsConfig().InitFromViper(v)
	if err != nil {
		// TODO refactor to be able to return error
		log.Fatal(err)
	}
}

// GetPrimary returns primary configuration.
func (opt *Options) GetPrimary() *config.Configuration {
	return &opt.Primary.Configuration
}

// Get returns auxiliary named configuration.
func (opt *Options) Get(namespace string) *config.Configuration {
	nsCfg, ok := opt.others[namespace]
	if !ok {
		nsCfg = &namespaceConfig{}
		opt.others[namespace] = nsCfg
	}
	nsCfg.Configuration.ApplyDefaults(&opt.Primary.Configuration)
	if len(nsCfg.Configuration.Servers) == 0 {
		nsCfg.Servers = opt.Primary.Servers
	}
	return &nsCfg.Configuration
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
		Username:                 "",
		Password:                 "",
		Sniffer:                  false,
		MaxSpanAge:               72 * time.Hour,
		AdaptiveSamplingLookback: 72 * time.Hour,
		BulkSize:                 5 * 1000 * 1000,
		BulkWorkers:              1,
		BulkActions:              1000,
		BulkFlushInterval:        time.Millisecond * 200,
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
		Indices: config.Indices{
			Spans:        defaultIndexOptions,
			Services:     defaultIndexOptions,
			Dependencies: defaultIndexOptions,
			Sampling:     defaultIndexOptions,
		},
	}
}
