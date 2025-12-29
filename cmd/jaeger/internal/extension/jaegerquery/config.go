// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/confmap/xconfmap"

	"github.com/jaegertracing/jaeger/cmd/internal/flags"
	queryapp "github.com/jaegertracing/jaeger/cmd/query/app"
)

var _ xconfmap.Validator = (*Config)(nil)

// Config represents the configuration for jaeger-query.
type Config struct {
	flags.QueryOptions `mapstructure:",squash"`
	// Storage holds configuration related to the various data stores that are to be queried.
	Storage Storage `mapstructure:"storage"`
}

type Storage struct {
	// TracesPrimary contains the name of the primary trace storage that is being queried.
	TracesPrimary string `mapstructure:"traces" valid:"required"`
	// TracesArchive contains the name of the archive trace storage that is being queried.
	TracesArchive string `mapstructure:"traces_archive" valid:"optional"`
	// Metrics contains the name of the metric storage that is being queried.
	Metrics string `mapstructure:"metrics" valid:"optional"`
}

// DefaultConfig creates the default configuration for jaeger-query.
func DefaultConfig() Config {
	return Config{
		QueryOptions: flags.DefaultQueryOptions(),
	}
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}

// ToQueryOptions converts the Config to the QueryOptions structure
// used by cmd/query/app package.
func (cfg *Config) ToQueryOptions() *queryapp.QueryOptions {
	return (*queryapp.QueryOptions)(&cfg.QueryOptions)
}
