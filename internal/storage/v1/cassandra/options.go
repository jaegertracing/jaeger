// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"strings"
	"time"

	"github.com/jaegertracing/jaeger/internal/config/tlscfg"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
)

// Options contains various type of Cassandra configs and provides the ability
// to bind them to command line flag and apply overlays, so that some configurations
// (e.g. archive) may be underspecified and infer the rest of its parameters from primary.
type Options struct {
	NamespaceConfig        `mapstructure:",squash"`
	SpanStoreWriteCacheTTL time.Duration `mapstructure:"span_store_write_cache_ttl"`
	Index                  IndexConfig   `mapstructure:"index"`
}

// IndexConfig configures indexing.
// By default all indexing is enabled.
type IndexConfig struct {
	Logs         bool   `mapstructure:"logs"`
	Tags         bool   `mapstructure:"tags"`
	ProcessTags  bool   `mapstructure:"process_tags"`
	TagBlackList string `mapstructure:"tag_blacklist"`
	TagWhiteList string `mapstructure:"tag_whitelist"`
}

// the Servers field in config.Configuration is a list, which we cannot represent with flags.
// This struct adds a plain string field that can be bound to flags and is then parsed when
// preparing the actual config.Configuration.
type NamespaceConfig struct {
	config.Configuration `mapstructure:",squash"`
	namespace            string
	Enabled              bool `mapstructure:"-"`
}

// NewOptions creates a new Options struct.
func NewOptions(namespace string) *Options {
	// TODO all default values should be defined via cobra flags
	options := &Options{
		NamespaceConfig: NamespaceConfig{
			Configuration: config.DefaultConfiguration(),
			namespace:     namespace,
			Enabled:       true,
		},
		SpanStoreWriteCacheTTL: time.Hour * 12,
	}

	return options
}

func tlsFlagsConfig(namespace string) tlscfg.ClientFlagsConfig {
	return tlscfg.ClientFlagsConfig{
		Prefix: namespace,
	}
}

func (opt *Options) GetConfig() config.Configuration {
	return opt.NamespaceConfig.Configuration
}

// TagIndexBlacklist returns the list of blacklisted tags
func (opt *Options) TagIndexBlacklist() []string {
	if opt.Index.TagBlackList != "" {
		return strings.Split(opt.Index.TagBlackList, ",")
	}

	return nil
}

// TagIndexWhitelist returns the list of whitelisted tags
func (opt *Options) TagIndexWhitelist() []string {
	if opt.Index.TagWhiteList != "" {
		return strings.Split(opt.Index.TagWhiteList, ",")
	}

	return nil
}

// stripWhiteSpace removes all whitespace characters from a string
func stripWhiteSpace(str string) string {
	return strings.ReplaceAll(str, " ", "")
}
