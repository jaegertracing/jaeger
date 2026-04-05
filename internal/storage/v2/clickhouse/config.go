// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"go.opentelemetry.io/collector/config/configoptional"
)

const (
	defaultProtocol                      = "native"
	defaultDatabase                      = "jaeger"
	defaultSearchDepth                   = 1000
	defaultMaxSearchDepth                = 10000
	defaultAttributeMetadataCacheTTL     = time.Hour
	defaultAttributeMetadataCacheMaxSize = 1000
)

type Configuration struct {
	// Protocol is the protocol to use to connect to ClickHouse.
	// Supported values are "native" and "http". Default is "native".
	Protocol string `mapstructure:"protocol" valid:"in(native|http),optional"`
	// Addresses contains a list of ClickHouse server addresses to connect to.
	Addresses []string `mapstructure:"addresses" valid:"required"`
	// Database is the ClickHouse database to connect to.
	Database string `mapstructure:"database"`
	// Auth contains the authentication configuration to connect to ClickHouse.
	Auth Authentication `mapstructure:"auth"`
	// DialTimeout is the timeout for establishing a connection to ClickHouse.
	DialTimeout time.Duration `mapstructure:"dial_timeout"`
	// CreateSchema, if set to true, will create the ClickHouse schema if it does not exist.
	CreateSchema bool `mapstructure:"create_schema"`
	// DefaultSearchDepth is the default search depth for queries.
	// This is the maximum number of trace IDs that will be returned when searching for traces
	// if a limit is not specified in the query.
	DefaultSearchDepth int `mapstructure:"default_search_depth"`
	// MaxSearchDepth is the maximum allowed search depth for queries.
	// This limits the number of trace IDs that can be returned when searching for traces.
	MaxSearchDepth int `mapstructure:"max_search_depth"`
	// AttributeMetadataCacheTTL is the time-to-live for cached attribute metadata entries.
	// Attribute metadata maps attribute keys to their stored types and levels,
	// which is needed to build type-correct queries for querying attributes.
	// Default is 1h.
	AttributeMetadataCacheTTL time.Duration `mapstructure:"attribute_metadata_cache_ttl"`
	// AttributeMetadataCacheMaxSize is the maximum number of entries in the attribute metadata cache.
	// Default is 1000.
	AttributeMetadataCacheMaxSize int `mapstructure:"attribute_metadata_cache_max_size"`
}

type Authentication struct {
	Basic configoptional.Optional[basicauthextension.ClientAuthSettings] `mapstructure:"basic"`
	// TODO: add JWT
}

func (cfg *Configuration) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}

func (cfg *Configuration) applyDefaults() {
	if cfg.Protocol == "" {
		cfg.Protocol = "native"
	}
	if cfg.Database == "" {
		cfg.Database = defaultDatabase
	}
	if cfg.DefaultSearchDepth == 0 {
		cfg.DefaultSearchDepth = defaultSearchDepth
	}
	if cfg.MaxSearchDepth == 0 {
		cfg.MaxSearchDepth = defaultMaxSearchDepth
	}
	if cfg.AttributeMetadataCacheTTL <= 0 {
		cfg.AttributeMetadataCacheTTL = defaultAttributeMetadataCacheTTL
	}
	if cfg.AttributeMetadataCacheMaxSize <= 0 {
		cfg.AttributeMetadataCacheMaxSize = defaultAttributeMetadataCacheMaxSize
	}
}
