// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap/xconfmap"
)

var (
	_ component.Config   = (*Config)(nil)
	_ xconfmap.Validator = (*Config)(nil)
)

// Config configures the jaeger_storage_writer connector. It writes each batch to the
// named trace storage exactly as jaeger_storage_exporter does, and taps the
// terminally-rejected ("poison") spans onto its connector output pipeline
// (RFC 0007 §4.8).
type Config struct {
	// TraceStorage names the storage backend (a jaeger_storage extension entry) the
	// connector writes to. The backend must run in synchronous write mode with
	// poison_pill_handling: fail so the writer returns the typed *BulkWriteError this
	// connector taps; under drop the writer discards poison itself and nothing is
	// dead-lettered.
	TraceStorage string `mapstructure:"trace_storage" valid:"required"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
