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
	"github.com/spf13/viper"
	"github.com/jaegertracing/jaeger/pkg/dashbase/config"
)

const (
	suffixKafkaHost  = ".kafkaHost"
	suffixServer     = ".dashbaseServer"
	suffixKafkaTopic = ".kafkaTopic"
)



// TODO this should be moved next to config.Configuration struct (maybe ./flags package)

// Options contains various type of ElasticSearch configs and provides the ability
// to bind them to command line flag and apply overlays, so that some configurations
// (e.g. archive) may be underspecified and infer the rest of its parameters from primary.
type Options struct {
	primary *namespaceConfig
	others  map[string]*namespaceConfig
}

// the Servers field in config.Configuration is a list, which we cannot represent with flags.
// This struct adds a plain string field that can be bound to flags and is then parsed when
// preparing the actual config.Configuration.
type namespaceConfig struct {
	config.Configuration
	kafkaHost string
	namespace string
}

// NewOptions creates a new Options struct.
func NewOptions(primaryNamespace string, otherNamespaces ...string) *Options {
	// TODO all default values should be defined via cobra flags
	options := &Options{
		primary: &namespaceConfig{
			Configuration: config.Configuration{
				Server: "http://127.0.0.1:9876",
			},
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
		nsConfig.namespace+suffixKafkaHost,
		nsConfig.kafkaHost,
		"The List of Kafka hosts required by Dashbase")
	flagSet.String(
		nsConfig.namespace+suffixKafkaTopic,
		nsConfig.KafkaTopic,
		"The Kafka Topic required by Dashbase")
	flagSet.String(
		nsConfig.namespace+suffixServer,
		nsConfig.Server,
		"The Dashbase API Host")
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	initFromViper(opt.primary, v)
	for _, cfg := range opt.others {
		initFromViper(cfg, v)
	}
}

func initFromViper(cfg *namespaceConfig, v *viper.Viper) {
	cfg.Server = v.GetString(cfg.namespace + suffixServer)
	cfg.kafkaHost = v.GetString(cfg.namespace + suffixKafkaHost)
	cfg.KafkaTopic = v.GetString(cfg.namespace + suffixKafkaTopic)
}

// GetPrimary returns primary configuration.
func (opt *Options) GetPrimary() *config.Configuration {
	opt.primary.KafkaHost = strings.Split(opt.primary.kafkaHost, ",")
	return &opt.primary.Configuration
}

// Get returns auxiliary named configuration.
func (opt *Options) Get(namespace string) *config.Configuration {
	nsCfg, ok := opt.others[namespace]
	if !ok {
		nsCfg = &namespaceConfig{}
		opt.others[namespace] = nsCfg
	}
	nsCfg.KafkaHost = strings.Split(nsCfg.kafkaHost, ",")
	return &nsCfg.Configuration
}
