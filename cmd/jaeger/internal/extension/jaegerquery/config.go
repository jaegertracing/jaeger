// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/confmap/xconfmap"

	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
)

var _ xconfmap.Validator = (*Config)(nil)

// Config represents the configuration for jaeger-query,
type Config struct {
	queryApp.QueryOptions `mapstructure:",squash"`
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

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
