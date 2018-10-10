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
)

const (
	suffixUsername          = ".username"
	suffixPassword          = ".password"
	suffixSniffer           = ".sniffer"
	suffixServerURLs        = ".server-urls"
	suffixMaxSpanAge        = ".max-span-age"
	suffixNumShards         = ".num-shards"
	suffixNumReplicas       = ".num-replicas"
	suffixBulkSize          = ".bulk.size"
	suffixBulkWorkers       = ".bulk.workers"
	suffixBulkActions       = ".bulk.actions"
	suffixBulkFlushInterval = ".bulk.flush-interval"
	suffixIndexPrefix       = ".index-prefix"
	suffixTagsAsFields      = ".tags-as-fields"
	suffixTagsAsFieldsAll   = suffixTagsAsFields + ".all"
	suffixTagsFile          = suffixTagsAsFields + ".config-file"
	suffixTagDeDotChar      = suffixTagsAsFields + ".dot-replacement"
)

// TODO this should be moved next to config.Configuration struct (maybe ./flags package)

// Options contains various type of ElasticSearch configs and provides the ability
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
				Username:          "",
				Password:          "",
				Sniffer:           false,
				MaxSpanAge:        72 * time.Hour,
				NumShards:         5,
				NumReplicas:       1,
				BulkSize:          5 * 1000 * 1000,
				BulkWorkers:       1,
				BulkActions:       1000,
				BulkFlushInterval: time.Millisecond * 200,
				TagDotReplacement: "@",
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
		"The username required by ElasticSearch")
	flagSet.String(
		nsConfig.namespace+suffixPassword,
		nsConfig.Password,
		"The password required by ElasticSearch")
	flagSet.Bool(
		nsConfig.namespace+suffixSniffer,
		nsConfig.Sniffer,
		"The sniffer config for ElasticSearch; client uses sniffing process to find all nodes automatically, disable if not required")
	flagSet.String(
		nsConfig.namespace+suffixServerURLs,
		nsConfig.servers,
		"The comma-separated list of ElasticSearch servers, must be full url i.e. http://localhost:9200")
	flagSet.Duration(
		nsConfig.namespace+suffixMaxSpanAge,
		nsConfig.MaxSpanAge,
		"The maximum lookback for spans in ElasticSearch")
	flagSet.Int64(
		nsConfig.namespace+suffixNumShards,
		nsConfig.NumShards,
		"The number of shards per index in ElasticSearch")
	flagSet.Int64(
		nsConfig.namespace+suffixNumReplicas,
		nsConfig.NumReplicas,
		"The number of replicas per index in ElasticSearch")
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
		"A time.Duration after which bulk requests are committed, regardless of other tresholds. Set to zero to disable. By default, this is disabled.")
	flagSet.String(
		nsConfig.namespace+suffixIndexPrefix,
		nsConfig.IndexPrefix,
		"Optional prefix of Jaeger indices. For example \"production\" creates \"production:jaeger-*\".")
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
	cfg.Sniffer = v.GetBool(cfg.namespace + suffixSniffer)
	cfg.servers = v.GetString(cfg.namespace + suffixServerURLs)
	cfg.MaxSpanAge = v.GetDuration(cfg.namespace + suffixMaxSpanAge)
	cfg.NumShards = v.GetInt64(cfg.namespace + suffixNumShards)
	cfg.NumReplicas = v.GetInt64(cfg.namespace + suffixNumReplicas)
	cfg.BulkSize = v.GetInt(cfg.namespace + suffixBulkSize)
	cfg.BulkWorkers = v.GetInt(cfg.namespace + suffixBulkWorkers)
	cfg.BulkActions = v.GetInt(cfg.namespace + suffixBulkActions)
	cfg.BulkFlushInterval = v.GetDuration(cfg.namespace + suffixBulkFlushInterval)
	cfg.IndexPrefix = v.GetString(cfg.namespace + suffixIndexPrefix)
	cfg.AllTagsAsFields = v.GetBool(cfg.namespace + suffixTagsAsFieldsAll)
	cfg.TagsFilePath = v.GetString(cfg.namespace + suffixTagsFile)
	cfg.TagDotReplacement = v.GetString(cfg.namespace + suffixTagDeDotChar)
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
