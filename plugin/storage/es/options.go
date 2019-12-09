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
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	suffixUsername            = ".username"
	suffixPassword            = ".password"
	suffixSniffer             = ".sniffer"
	suffixTokenPath           = ".token-file"
	suffixServerURLs          = ".server-urls"
	suffixMaxSpanAge          = ".max-span-age"
	suffixMaxNumSpans         = ".max-num-spans"
	suffixNumShards           = ".num-shards"
	suffixNumReplicas         = ".num-replicas"
	suffixBulkSize            = ".bulk.size"
	suffixBulkWorkers         = ".bulk.workers"
	suffixBulkActions         = ".bulk.actions"
	suffixBulkFlushInterval   = ".bulk.flush-interval"
	suffixTimeout             = ".timeout"
	suffixTLS                 = ".tls"
	suffixCert                = ".tls.cert"
	suffixKey                 = ".tls.key"
	suffixCA                  = ".tls.ca"
	suffixSkipHostVerify      = ".tls.skip-host-verify"
	suffixIndexPrefix         = ".index-prefix"
	suffixTagsAsFields        = ".tags-as-fields"
	suffixTagsAsFieldsAll     = suffixTagsAsFields + ".all"
	suffixTagsFile            = suffixTagsAsFields + ".config-file"
	suffixTagDeDotChar        = suffixTagsAsFields + ".dot-replacement"
	suffixReadAlias           = ".use-aliases"
	suffixCreateIndexTemplate = ".create-index-templates"
	suffixEnabled             = ".enabled"
	suffixVersion             = ".version"
)

// TODO this should be moved next to config.Configuration struct (maybe ./flags package)

// Options contains various type of Elasticsearch configs and provides the ability
// to bind them to command line flag and apply overlays, so that some configurations
// (e.g. archive) may be underspecified and infer the rest of its parameters from primary.
type Options struct {
	primary *namespaceConfig

	others map[string]*namespaceConfig
}

// the Servers field in config.Configuration is a list, which we cannot represent with flags.
// This struct adds a plain string field that can be bound to flags and is then parsed when
// preparing the actual config.Configuration.
type namespaceConfig struct {
	config.Configuration
	servers   string
	namespace string
}

// NewOptions creates a new Options struct.
func NewOptions(primaryNamespace string, otherNamespaces ...string) *Options {
	// TODO all default values should be defined via cobra flags
	options := &Options{
		primary: &namespaceConfig{
			Configuration: config.Configuration{
				Username:             "",
				Password:             "",
				Sniffer:              false,
				MaxNumSpans:          10000,
				NumShards:            5,
				NumReplicas:          1,
				BulkSize:             5 * 1000 * 1000,
				BulkWorkers:          1,
				BulkActions:          1000,
				BulkFlushInterval:    time.Millisecond * 200,
				TagDotReplacement:    "@",
				Enabled:              true,
				CreateIndexTemplates: true,
				Version:              0,
			},
			servers:   "http://127.0.0.1:9200",
			namespace: primaryNamespace,
		},
		others: make(map[string]*namespaceConfig, len(otherNamespaces)),
	}

	for _, namespace := range otherNamespaces {
		options.others[namespace] = &namespaceConfig{namespace: namespace}
	}

	return options
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	addFlags(flagSet, opt.primary)
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
	flagSet.Bool(
		nsConfig.namespace+suffixSniffer,
		nsConfig.Sniffer,
		"The sniffer config for Elasticsearch; client uses sniffing process to find all nodes automatically, disable if not required")
	flagSet.String(
		nsConfig.namespace+suffixServerURLs,
		nsConfig.servers,
		"The comma-separated list of Elasticsearch servers, must be full url i.e. http://localhost:9200")
	flagSet.Duration(
		nsConfig.namespace+suffixTimeout,
		nsConfig.Timeout,
		"Timeout used for queries. A Timeout of zero means no timeout")
	flagSet.Duration(
		nsConfig.namespace+suffixMaxSpanAge,
		time.Hour*72,
		"(deprecated) The maximum lookback for spans in Elasticsearch. Now all indices are searched.")
	flagSet.Int(
		nsConfig.namespace+suffixMaxNumSpans,
		nsConfig.MaxNumSpans,
		"The maximum number of spans to fetch at a time per query in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffixNumShards,
		nsConfig.NumShards,
		"The number of shards per index in Elasticsearch")
	flagSet.Int64(
		nsConfig.namespace+suffixNumReplicas,
		nsConfig.NumReplicas,
		"The number of replicas per index in Elasticsearch")
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
	flagSet.Bool(
		nsConfig.namespace+suffixTLS,
		nsConfig.TLS.Enabled,
		"Enable TLS with client certificates.")
	flagSet.Bool(
		nsConfig.namespace+suffixSkipHostVerify,
		nsConfig.TLS.SkipHostVerify,
		"(insecure) Skip server's certificate chain and host name verification")
	flagSet.String(
		nsConfig.namespace+suffixCert,
		nsConfig.TLS.CertPath,
		"Path to TLS certificate file")
	flagSet.String(
		nsConfig.namespace+suffixKey,
		nsConfig.TLS.KeyPath,
		"Path to TLS key file")
	flagSet.String(
		nsConfig.namespace+suffixCA,
		nsConfig.TLS.CaPath,
		"Path to TLS CA file")
	flagSet.String(
		nsConfig.namespace+suffixIndexPrefix,
		nsConfig.IndexPrefix,
		"Optional prefix of Jaeger indices. For example \"production\" creates \"production-jaeger-*\".")
	flagSet.Bool(
		nsConfig.namespace+suffixTagsAsFieldsAll,
		nsConfig.AllTagsAsFields,
		"(experimental) Store all span and process tags as object fields. If true "+suffixTagsFile+" is ignored. Binary tags are always stored as nested objects.")
	flagSet.String(
		nsConfig.namespace+suffixTagsFile,
		nsConfig.TagsFilePath,
		"(experimental) Optional path to a file containing tag keys which will be stored as object fields. Each key should be on a separate line.")
	flagSet.String(
		nsConfig.namespace+suffixTagDeDotChar,
		nsConfig.TagDotReplacement,
		"(experimental) The character used to replace dots (\".\") in tag keys stored as object fields.")
	flagSet.Bool(
		nsConfig.namespace+suffixReadAlias,
		nsConfig.UseReadWriteAliases,
		"(experimental) Use read and write aliases for indices. Use this option with Elasticsearch rollover "+
			"API. It requires an external component to create aliases before startup and then performing its management.")
	flagSet.Bool(
		nsConfig.namespace+suffixCreateIndexTemplate,
		nsConfig.CreateIndexTemplates,
		"Create index templates at application startup. Set to false when templates are installed manually.")
	flagSet.Uint(
		nsConfig.namespace+suffixVersion,
		0,
		"The major Elasticsearch version. If not specified, the value will be auto-detected from Elasticsearch.")
	if nsConfig.namespace == archiveNamespace {
		flagSet.Bool(
			nsConfig.namespace+suffixEnabled,
			nsConfig.Enabled,
			"Enable extra storage")
	}
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	initFromViper(opt.primary, v)
	for _, cfg := range opt.others {
		initFromViper(cfg, v)
	}
}

func initFromViper(cfg *namespaceConfig, v *viper.Viper) {
	cfg.Username = v.GetString(cfg.namespace + suffixUsername)
	cfg.Password = v.GetString(cfg.namespace + suffixPassword)
	cfg.TokenFilePath = v.GetString(cfg.namespace + suffixTokenPath)
	cfg.Sniffer = v.GetBool(cfg.namespace + suffixSniffer)
	cfg.servers = stripWhiteSpace(v.GetString(cfg.namespace + suffixServerURLs))
	cfg.MaxNumSpans = v.GetInt(cfg.namespace + suffixMaxNumSpans)
	cfg.NumShards = v.GetInt64(cfg.namespace + suffixNumShards)
	cfg.NumReplicas = v.GetInt64(cfg.namespace + suffixNumReplicas)
	cfg.BulkSize = v.GetInt(cfg.namespace + suffixBulkSize)
	cfg.BulkWorkers = v.GetInt(cfg.namespace + suffixBulkWorkers)
	cfg.BulkActions = v.GetInt(cfg.namespace + suffixBulkActions)
	cfg.BulkFlushInterval = v.GetDuration(cfg.namespace + suffixBulkFlushInterval)
	cfg.Timeout = v.GetDuration(cfg.namespace + suffixTimeout)
	cfg.TLS.Enabled = v.GetBool(cfg.namespace + suffixTLS)
	cfg.TLS.SkipHostVerify = v.GetBool(cfg.namespace + suffixSkipHostVerify)
	cfg.TLS.CertPath = v.GetString(cfg.namespace + suffixCert)
	cfg.TLS.KeyPath = v.GetString(cfg.namespace + suffixKey)
	cfg.TLS.CaPath = v.GetString(cfg.namespace + suffixCA)
	cfg.IndexPrefix = v.GetString(cfg.namespace + suffixIndexPrefix)
	cfg.AllTagsAsFields = v.GetBool(cfg.namespace + suffixTagsAsFieldsAll)
	cfg.TagsFilePath = v.GetString(cfg.namespace + suffixTagsFile)
	cfg.TagDotReplacement = v.GetString(cfg.namespace + suffixTagDeDotChar)
	cfg.UseReadWriteAliases = v.GetBool(cfg.namespace + suffixReadAlias)
	cfg.Enabled = v.GetBool(cfg.namespace + suffixEnabled)
	cfg.CreateIndexTemplates = v.GetBool(cfg.namespace + suffixCreateIndexTemplate)
	cfg.Version = uint(v.GetInt(cfg.namespace + suffixVersion))
	// TODO: Need to figure out a better way for do this.
	cfg.AllowTokenFromContext = v.GetBool(spanstore.StoragePropagationKey)
}

// GetPrimary returns primary configuration.
func (opt *Options) GetPrimary() *config.Configuration {
	opt.primary.Servers = strings.Split(opt.primary.servers, ",")
	return &opt.primary.Configuration
}

// Get returns auxiliary named configuration.
func (opt *Options) Get(namespace string) *config.Configuration {
	nsCfg, ok := opt.others[namespace]
	if !ok {
		nsCfg = &namespaceConfig{}
		opt.others[namespace] = nsCfg
	}
	nsCfg.Configuration.ApplyDefaults(&opt.primary.Configuration)
	if nsCfg.servers == "" {
		nsCfg.servers = opt.primary.servers
	}
	nsCfg.Servers = strings.Split(nsCfg.servers, ",")
	return &nsCfg.Configuration
}

// stripWhiteSpace removes all whitespace characters from a string
func stripWhiteSpace(str string) string {
	return strings.Replace(str, " ", "", -1)
}
