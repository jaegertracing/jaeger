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
	suffix                             = "."
	username                           = "username"
	password                           = "password"
	sniffer                            = "sniffer"
	snifferTLSEnabled                  = "sniffer-tls-enabled"
	tokenPath                          = "token-file"
	passwordPath                       = "password-file"
	serverURLs                         = "server-urls"
	remoteReadClusters                 = "remote-read-clusters"
	maxSpanAge                         = "max-span-age"
	adaptiveSamplingLookback           = "adaptive-sampling.lookback"
	numShards                          = "num-shards"
	numReplicas                        = "num-replicas"
	prioritySpanTemplate               = "prioirity-span-template"
	priorityServiceTemplate            = "prioirity-service-template"
	priorityDependenciesTemplate       = "prioirity-dependencies-template"
	bulkSize                           = "bulk.size"
	bulkWorkers                        = "bulk.workers"
	bulkActions                        = "bulk.actions"
	bulkFlushInterval                  = "bulk.flush-interval"
	timeout                            = "timeout"
	indexPrefix                        = "index-prefix"
	indexDateSeparator                 = "index-date-separator"
	indexRolloverFrequencySpans        = "index-rollover-frequency-spans"
	indexRolloverFrequencyServices     = "index-rollover-frequency-services"
	indexRolloverFrequencySampling     = "index-rollover-frequency-adaptive-sampling"
	indexRolloverFrequencyDependencies = "index-rollover-frequency-adaptive-dependencies"
	serviceCacheTTL                    = "service-cache-ttl"
	tagsAsFields                       = "tags-as-fields"
	tagsAsFieldsAll                    = tagsAsFields + ".all"
	tagsAsFieldsInclude                = tagsAsFields + ".include"
	tagsFile                           = tagsAsFields + ".config-file"
	tagDeDotChar                       = tagsAsFields + ".dot-replacement"
	readAlias                          = "use-aliases"
	useILM                             = "use-ilm"
	createIndexTemplate                = "create-index-templates"
	enabled                            = "enabled"
	version                            = "version"
	maxDocCount                        = "max-doc-count"
	logLevel                           = "log-level"
	sendGetBodyAs                      = "send-get-body-as"
	// default number of documents to return from a query (elasticsearch allowed limit)
	// see search.max_buckets and index.max_result_window
	defaultMaxDocCount        = 10_000
	defaultServerURL          = "http://127.0.0.1:9200"
	defaultRemoteReadClusters = ""
	// default separator for Elasticsearch index date layout.
	defaultIndexDateSeparator = "-"

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

func flagWithSuffix(namespace, flag string) string {
	if namespace != "" {
		return namespace + suffix + flag
	} else {
		return suffix + flag
	}
}

func addFlags(flagSet *flag.FlagSet, nsConfig *namespaceConfig) {
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, username),
		nsConfig.Username,
		"The username required by Elasticsearch. The basic authentication also loads CA if it is specified.")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, password),
		nsConfig.Password,
		"The password required by Elasticsearch")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, tokenPath),
		nsConfig.TokenFilePath,
		"Path to a file containing bearer token. This flag also loads CA if it is specified.")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, passwordPath),
		nsConfig.PasswordFilePath,
		"Path to a file containing password. This file is watched for changes.")
	flagSet.Bool(
		flagWithSuffix(nsConfig.namespace, sniffer),
		nsConfig.Sniffer,
		"The sniffer config for Elasticsearch; client uses sniffing process to find all nodes automatically, disable if not required")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, serverURLs),
		defaultServerURL,
		"The comma-separated list of Elasticsearch servers, must be full url i.e. http://localhost:9200")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, remoteReadClusters),
		defaultRemoteReadClusters,
		"Comma-separated list of Elasticsearch remote cluster names for cross-cluster querying."+
			"See Elasticsearch remote clusters and cross-cluster query api.")
	flagSet.Duration(
		flagWithSuffix(nsConfig.namespace, timeout),
		nsConfig.Timeout,
		"Timeout used for queries. A Timeout of zero means no timeout")
	// TODO deprecated flag to be removed
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, numShards),
		defaultIndexOptions.TemplateNumShards,
		"(deprecated, will be removed in the future, use .num-shards-spans or .num-shards-services or .num-shards-sampling or .num-shards-dependencies instead) The number of shards per index in Elasticsearch")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, numReplicas),
		defaultIndexOptions.TemplateNumReplicas,
		"(deprecated, will be removed in the future, use .num-replicas-spans or .num-replicas-services or .num-replicas-sampling or .num-replicas-dependencies instead) The number of replicas per index in Elasticsearch")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, config.GetNumShardSpanFlag()),
		nsConfig.Indices.Spans.TemplateNumShards,
		"The number of shards per span index in Elasticsearch")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, config.GetNumShardServiceFlag()),
		nsConfig.Indices.Services.TemplateNumShards,
		"The number of shards per service index in Elasticsearch")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, config.GetNumShardSamplingFlag()),
		nsConfig.Indices.Sampling.TemplateNumShards,
		"The number of shards per sampling index in Elasticsearch")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, config.GetNumShardDependenciesFlag()),
		nsConfig.Indices.Dependencies.TemplateNumShards,
		"The number of shards per dependencies index in Elasticsearch")
	flagSet.Duration(
		flagWithSuffix(nsConfig.namespace, serviceCacheTTL),
		nsConfig.ServiceCacheTTL,
		"The TTL for the cache of known service names",
	)
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, config.GetNumReplicasSpanFlag()),
		nsConfig.Indices.Spans.TemplateNumReplicas,
		"The number of replicas per span index in Elasticsearch")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, config.GetNumReplicasServiceFlag()),
		nsConfig.Indices.Services.TemplateNumReplicas,
		"The number of replicas per service index in Elasticsearch")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, config.GetNumReplicasSamplingFlag()),
		nsConfig.Indices.Sampling.TemplateNumReplicas,
		"The number of replicas per sampling index in Elasticsearch")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, config.GetNumReplicasDependenciesFlag()),
		nsConfig.Indices.Dependencies.TemplateNumReplicas,
		"The number of replicas per dependencies index in Elasticsearch")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, prioritySpanTemplate),
		nsConfig.Indices.Spans.TemplatePriority,
		"Priority of jaeger-span index template (ESv8 only)")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, priorityServiceTemplate),
		nsConfig.Indices.Services.TemplatePriority,
		"Priority of jaeger-service index template (ESv8 only)")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, priorityDependenciesTemplate),
		nsConfig.Indices.Dependencies.TemplatePriority,
		"Priority of jaeger-dependecies index template (ESv8 only)")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, bulkSize),
		nsConfig.BulkSize,
		"The number of bytes that the bulk requests can take up before the bulk processor decides to commit")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, bulkWorkers),
		nsConfig.BulkWorkers,
		"The number of workers that are able to receive bulk requests and eventually commit them to Elasticsearch")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, bulkActions),
		nsConfig.BulkActions,
		"The number of requests that can be enqueued before the bulk processor decides to commit")
	flagSet.Duration(
		flagWithSuffix(nsConfig.namespace, bulkFlushInterval),
		nsConfig.BulkFlushInterval,
		"A time.Duration after which bulk requests are committed, regardless of other thresholds. Set to zero to disable. By default, this is disabled.")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, indexPrefix),
		nsConfig.IndexPrefix,
		"Optional prefix of Jaeger indices. For example \"production\" creates \"production-jaeger-*\".")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, indexDateSeparator),
		defaultIndexDateSeparator,
		"Optional date separator of Jaeger indices. For example \".\" creates \"jaeger-span-2020.11.20\".")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, indexRolloverFrequencySpans),
		defaultIndexRolloverFrequency,
		"Rotates jaeger-span indices over the given period. For example \"day\" creates \"jaeger-span-yyyy-MM-dd\" every day after UTC 12AM. Valid options: [hour, day]. "+
			"This does not delete old indices. For details on complete index management solutions supported by Jaeger, refer to: https://www.jaegertracing.io/docs/deployment/#elasticsearch-rollover")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, indexRolloverFrequencyServices),
		defaultIndexRolloverFrequency,
		"Rotates jaeger-service indices over the given period. For example \"day\" creates \"jaeger-service-yyyy-MM-dd\" every day after UTC 12AM. Valid options: [hour, day]. "+
			"This does not delete old indices. For details on complete index management solutions supported by Jaeger, refer to: https://www.jaegertracing.io/docs/deployment/#elasticsearch-rollover")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, indexRolloverFrequencySampling),
		defaultIndexRolloverFrequency,
		"Rotates jaeger-sampling indices over the given period. For example \"day\" creates \"jaeger-sampling-yyyy-MM-dd\" every day after UTC 12AM. Valid options: [hour, day]. "+
			"This does not delete old indices. For details on complete index management solutions supported by Jaeger, refer to: https://www.jaegertracing.io/docs/deployment/#elasticsearch-rollover")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, indexRolloverFrequencyDependencies),
		defaultIndexRolloverFrequency,
		"Rotates jaeger-dependencies indices over the given period. For example \"day\" creates \"jaeger-dependencies-yyyy-MM-dd\" every day after UTC 12AM. Valid options: [hour, day]. "+
			"This does not delete old indices. For details on complete index management solutions supported by Jaeger, refer to: https://www.jaegertracing.io/docs/deployment/#elasticsearch-rollover")
	flagSet.Bool(
		flagWithSuffix(nsConfig.namespace, tagsAsFieldsAll),
		nsConfig.Tags.AllAsFields,
		"(experimental) Store all span and process tags as object fields. If true "+tagsFile+" and "+tagsAsFieldsInclude+" is ignored. Binary tags are always stored as nested objects.")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, tagsAsFieldsInclude),
		nsConfig.Tags.Include,
		"(experimental) Comma delimited list of tag keys which will be stored as object fields. Merged with the contents of "+tagsFile)
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, tagsFile),
		nsConfig.Tags.File,
		"(experimental) Optional path to a file containing tag keys which will be stored as object fields. Each key should be on a separate line.  Merged with "+tagsAsFieldsInclude)
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, tagDeDotChar),
		nsConfig.Tags.DotReplacement,
		"(experimental) The character used to replace dots (\".\") in tag keys stored as object fields.")
	flagSet.Bool(
		flagWithSuffix(nsConfig.namespace, readAlias),
		nsConfig.UseReadWriteAliases,
		"Use read and write aliases for indices. Use this option with Elasticsearch rollover "+
			"API. It requires an external component to create aliases before startup and then performing its management. "+
			"Note that es"+maxSpanAge+" will influence trace search window start times.")
	flagSet.Bool(
		flagWithSuffix(nsConfig.namespace, useILM),
		nsConfig.UseILM,
		"(experimental) Option to enable ILM for jaeger span & service indices. Use this option with  "+nsConfig.namespace+readAlias+". "+
			"It requires an external component to create aliases before startup and then performing its management. "+
			"ILM policy must be manually created in ES before startup. Supported only for elasticsearch version 7+.")
	flagSet.Bool(
		flagWithSuffix(nsConfig.namespace, createIndexTemplate),
		nsConfig.CreateIndexTemplates,
		"Create index templates at application startup. Set to false when templates are installed manually.")
	flagSet.Uint(
		flagWithSuffix(nsConfig.namespace, version),
		0,
		"The major Elasticsearch version. If not specified, the value will be auto-detected from Elasticsearch.")
	flagSet.Bool(
		flagWithSuffix(nsConfig.namespace, snifferTLSEnabled),
		nsConfig.SnifferTLSEnabled,
		"Option to enable TLS when sniffing an Elasticsearch Cluster ; client uses sniffing process to find all nodes automatically, disabled by default")
	flagSet.Int(
		flagWithSuffix(nsConfig.namespace, maxDocCount),
		nsConfig.MaxDocCount,
		"The maximum document count to return from an Elasticsearch query. This will also apply to aggregations.")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, logLevel),
		nsConfig.LogLevel,
		"The Elasticsearch client log-level. Valid levels: [debug, info, error]")
	flagSet.String(
		flagWithSuffix(nsConfig.namespace, sendGetBodyAs),
		nsConfig.SendGetBodyAs,
		"HTTP verb for requests that contain a body [GET, POST].")
	flagSet.Duration(
		flagWithSuffix(nsConfig.namespace, adaptiveSamplingLookback),
		nsConfig.AdaptiveSamplingLookback,
		"How far back to look for the latest adaptive sampling probabilities")
	if nsConfig.namespace == archiveNamespace {
		flagSet.Bool(
			flagWithSuffix(nsConfig.namespace, enabled),
			nsConfig.Enabled,
			"Enable extra storage")
	} else {
		// MaxSpanAge is only relevant when searching for unarchived traces.
		// Archived traces are searched with no look-back limit.
		flagSet.Duration(
			flagWithSuffix(nsConfig.namespace, maxSpanAge),
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
	overrideIndexShardsNums := func(deprecatedNum, newNum int) int {
		if deprecatedNum > 0 {
			return deprecatedNum
		} else {
			return newNum
		}
	}
	cfg.Username = v.GetString(flagWithSuffix(cfg.namespace, username))
	cfg.Password = v.GetString(flagWithSuffix(cfg.namespace, password))
	cfg.TokenFilePath = v.GetString(flagWithSuffix(cfg.namespace, tokenPath))
	cfg.PasswordFilePath = v.GetString(flagWithSuffix(cfg.namespace, passwordPath))
	cfg.Sniffer = v.GetBool(flagWithSuffix(cfg.namespace, sniffer))
	cfg.SnifferTLSEnabled = v.GetBool(flagWithSuffix(cfg.namespace, snifferTLSEnabled))
	cfg.Servers = strings.Split(stripWhiteSpace(v.GetString(flagWithSuffix(cfg.namespace, serverURLs))), ",")
	cfg.MaxSpanAge = v.GetDuration(flagWithSuffix(cfg.namespace, maxSpanAge))
	cfg.AdaptiveSamplingLookback = v.GetDuration(flagWithSuffix(cfg.namespace, adaptiveSamplingLookback))

	deprecatedNumShards := v.GetInt(flagWithSuffix(cfg.namespace, numShards))
	deprecatedReplicaShards := v.GetInt(flagWithSuffix(cfg.namespace, numReplicas))

	cfg.Indices.Spans.TemplateNumShards = overrideIndexShardsNums(deprecatedNumShards, v.GetInt(flagWithSuffix(cfg.namespace, config.GetNumShardSpanFlag())))
	cfg.Indices.Services.TemplateNumShards = overrideIndexShardsNums(deprecatedNumShards, v.GetInt(flagWithSuffix(cfg.namespace, config.GetNumShardServiceFlag())))
	cfg.Indices.Sampling.TemplateNumShards = overrideIndexShardsNums(deprecatedNumShards, v.GetInt(flagWithSuffix(cfg.namespace, config.GetNumShardSamplingFlag())))
	cfg.Indices.Dependencies.TemplateNumShards = overrideIndexShardsNums(deprecatedNumShards, v.GetInt(flagWithSuffix(cfg.namespace, config.GetNumShardDependenciesFlag())))

	cfg.Indices.Spans.TemplateNumReplicas = overrideIndexShardsNums(deprecatedReplicaShards, v.GetInt(flagWithSuffix(cfg.namespace, config.GetNumReplicasSpanFlag())))
	cfg.Indices.Services.TemplateNumReplicas = overrideIndexShardsNums(deprecatedReplicaShards, v.GetInt(flagWithSuffix(cfg.namespace, config.GetNumReplicasServiceFlag())))
	cfg.Indices.Sampling.TemplateNumReplicas = overrideIndexShardsNums(deprecatedReplicaShards, v.GetInt(flagWithSuffix(cfg.namespace, config.GetNumReplicasSamplingFlag())))
	cfg.Indices.Dependencies.TemplateNumReplicas = overrideIndexShardsNums(deprecatedReplicaShards, v.GetInt(flagWithSuffix(cfg.namespace, config.GetNumReplicasDependenciesFlag())))

	cfg.Indices.Spans.TemplatePriority = v.GetInt(flagWithSuffix(cfg.namespace, prioritySpanTemplate))
	cfg.Indices.Services.TemplatePriority = v.GetInt(flagWithSuffix(cfg.namespace, priorityServiceTemplate))
	cfg.Indices.Dependencies.TemplatePriority = v.GetInt(flagWithSuffix(cfg.namespace, priorityDependenciesTemplate))

	cfg.BulkSize = v.GetInt(flagWithSuffix(cfg.namespace, bulkSize))
	cfg.BulkWorkers = v.GetInt(flagWithSuffix(cfg.namespace, bulkWorkers))
	cfg.BulkActions = v.GetInt(flagWithSuffix(cfg.namespace, bulkActions))
	cfg.BulkFlushInterval = v.GetDuration(flagWithSuffix(cfg.namespace, bulkFlushInterval))
	cfg.Timeout = v.GetDuration(flagWithSuffix(cfg.namespace, timeout))
	cfg.ServiceCacheTTL = v.GetDuration(flagWithSuffix(cfg.namespace, serviceCacheTTL))
	cfg.IndexPrefix = v.GetString(flagWithSuffix(cfg.namespace, indexPrefix))
	cfg.Tags.AllAsFields = v.GetBool(flagWithSuffix(cfg.namespace, tagsAsFieldsAll))
	cfg.Tags.Include = v.GetString(flagWithSuffix(cfg.namespace, tagsAsFieldsInclude))
	cfg.Tags.File = v.GetString(flagWithSuffix(cfg.namespace, tagsFile))
	cfg.Tags.DotReplacement = v.GetString(flagWithSuffix(cfg.namespace, tagDeDotChar))
	cfg.UseReadWriteAliases = v.GetBool(flagWithSuffix(cfg.namespace, readAlias))
	cfg.Enabled = v.GetBool(flagWithSuffix(cfg.namespace, enabled))
	cfg.CreateIndexTemplates = v.GetBool(flagWithSuffix(cfg.namespace, createIndexTemplate))
	cfg.Version = uint(v.GetInt(flagWithSuffix(cfg.namespace, version)))
	cfg.LogLevel = v.GetString(flagWithSuffix(cfg.namespace, logLevel))
	cfg.SendGetBodyAs = v.GetString(flagWithSuffix(cfg.namespace, sendGetBodyAs))

	cfg.MaxDocCount = v.GetInt(flagWithSuffix(cfg.namespace, maxDocCount))
	cfg.UseILM = v.GetBool(flagWithSuffix(cfg.namespace, useILM))

	// TODO: Need to figure out a better way for do this.
	cfg.AllowTokenFromContext = v.GetBool(bearertoken.StoragePropagationKey)

	rReadClusters := stripWhiteSpace(v.GetString(flagWithSuffix(cfg.namespace, remoteReadClusters)))
	if len(rReadClusters) > 0 {
		cfg.RemoteReadClusters = strings.Split(rReadClusters, ",")
	}

	cfg.Indices.Spans.RolloverFrequency = strings.ToLower(v.GetString(flagWithSuffix(cfg.namespace, indexRolloverFrequencySpans)))
	cfg.Indices.Services.RolloverFrequency = strings.ToLower(v.GetString(flagWithSuffix(cfg.namespace, indexRolloverFrequencyServices)))
	cfg.Indices.Sampling.RolloverFrequency = strings.ToLower(v.GetString(flagWithSuffix(cfg.namespace, indexRolloverFrequencySampling)))

	separator := v.GetString(flagWithSuffix(cfg.namespace, indexDateSeparator))
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
