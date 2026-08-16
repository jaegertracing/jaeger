// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"errors"
	"fmt"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
)

const (
	defaultProtocol                        = "native"
	defaultDatabase                        = "jaeger"
	defaultSearchDepth                     = 1000
	defaultMaxSearchDepth                  = 10000
	defaultAttributeMetadataCacheTTL       = time.Hour
	defaultAttributeMetadataCacheMaxSize   = 1000
	defaultTraceIDBloomFilterFalsePositive = 0.025
	minTraceIDBloomFilterFalsePositive     = 1e-7
	maxTraceIDBloomFilterFalsePositive     = 0.1
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
	// TLS, when present, enables TLS for the ClickHouse connection.
	TLS configoptional.Optional[configtls.ClientConfig] `mapstructure:"tls"`
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
	// TTL is the Time-To-Live for spans in the database.
	// Data older than this will be automatically deleted. 0 means disabled.
	TTL time.Duration `mapstructure:"ttl"`
	// TraceIDBloomFilterFalsePositive is the false-positive rate for the
	// bloom_filter skip index on spans.trace_id. It only affects schema
	// creation (create_schema: true); existing tables are not altered.
	// Default is 0.025 (ClickHouse's implicit default). For high-scale
	// deployments where FindTraces is limited by trace-ID filtering,
	// operators may set this to 0.0001.
	TraceIDBloomFilterFalsePositive *float64 `mapstructure:"trace_id_bloom_filter_false_positive"`
}

type Authentication struct {
	Basic configoptional.Optional[basicauthextension.ClientAuthSettings] `mapstructure:"basic"`
}

func (cfg *Configuration) Validate() error {
	if _, err := govalidator.ValidateStruct(cfg); err != nil {
		return err
	}
	if cfg.TTL < 0 {
		return errors.New("ttl must be a non-negative duration")
	}
	if cfg.TTL > 0 && cfg.TTL%time.Second != 0 {
		return errors.New("ttl must be a whole number of seconds")
	}
	// Nil is valid: applyDefaults fills in the ClickHouse default before schema creation.
	if cfg.TraceIDBloomFilterFalsePositive != nil {
		fp := *cfg.TraceIDBloomFilterFalsePositive
		if fp <= 0 || fp >= 1 {
			return errors.New("trace_id_bloom_filter_false_positive must be between 0 and 1")
		}
		if fp < minTraceIDBloomFilterFalsePositive || fp > maxTraceIDBloomFilterFalsePositive {
			return fmt.Errorf(
				"trace_id_bloom_filter_false_positive must be between %g and %g",
				minTraceIDBloomFilterFalsePositive,
				maxTraceIDBloomFilterFalsePositive,
			)
		}
	}
	return nil
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
	if cfg.TraceIDBloomFilterFalsePositive == nil {
		v := defaultTraceIDBloomFilterFalsePositive
		cfg.TraceIDBloomFilterFalsePositive = &v
	}
}
