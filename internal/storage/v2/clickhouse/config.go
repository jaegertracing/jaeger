// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"errors"
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
	Protocol                      string         `mapstructure:"protocol" valid:"in(native|http),optional"`
	Addresses                     []string       `mapstructure:"addresses" valid:"required"`
	Database                      string         `mapstructure:"database"`
	Auth                          Authentication `mapstructure:"auth"`
	DialTimeout                   time.Duration  `mapstructure:"dial_timeout"`
	CreateSchema                  bool           `mapstructure:"create_schema"`
	DefaultSearchDepth            int            `mapstructure:"default_search_depth"`
	MaxSearchDepth                int            `mapstructure:"max_search_depth"`
	AttributeMetadataCacheTTL     time.Duration  `mapstructure:"attribute_metadata_cache_ttl"`
	AttributeMetadataCacheMaxSize int            `mapstructure:"attribute_metadata_cache_max_size"`
	// TTL is the Time-To-Live for spans in the database.
	// Data older than this will be automatically deleted. 0 means disabled.
	TTL time.Duration `mapstructure:"ttl"`
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
}
