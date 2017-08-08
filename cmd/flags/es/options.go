// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package es

import (
	"flag"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/uber/jaeger/pkg/es/config"
)

const (
	suffixUsername    = ".username"
	suffixPassword    = ".password"
	suffixSniffer     = ".sniffer"
	suffixServerURLs  = ".server-urls"
	suffixMaxSpanAge  = ".max-span-age"
	suffixNumShards   = ".num-shards"
	suffixNumReplicas = ".num-replicas"
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
	options := &Options{
		primary: &namespaceConfig{
			Configuration: config.Configuration{
				Username:    "",
				Password:    "",
				Sniffer:     false,
				MaxSpanAge:  72 * time.Hour,
				NumShards:   5,
				NumReplicas: 2,
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
