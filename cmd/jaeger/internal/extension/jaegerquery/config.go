// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"fmt"
	"path"
	"strings"

	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/confmap/xconfmap"

	queryapp "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal"
)

var _ xconfmap.Validator = (*Config)(nil)

// Config represents the configuration for jaeger-query,
type Config struct {
	queryapp.QueryOptions `mapstructure:",squash"`
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
	// Normalize BasePath once so all downstream consumers see a clean value.
	bp := cfg.BasePath
	if bp != "" && bp != "/" {
		if !strings.HasPrefix(bp, "/") {
			return fmt.Errorf("invalid base_path %q: must start with '/'", bp)
		}
		// path.Clean collapses duplicate slashes and resolves . / .. segments,
		// producing a canonical path without a trailing slash.
		clean := path.Clean(bp)
		if clean != bp && clean+"/" != bp {
			// The value had duplicate slashes or traversal segments beyond a
			// single trailing slash — reject it so callers don't silently get
			// a different path than they intended.
			return fmt.Errorf("invalid base_path %q: must not contain dot segments, path traversal, or duplicate slashes (normalized: %q)", bp, clean)
		}
		cfg.BasePath = clean
	}
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
