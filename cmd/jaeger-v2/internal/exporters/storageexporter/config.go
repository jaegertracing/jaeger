// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/component"
)

var (
	_ component.Config          = (*Config)(nil)
	_ component.ConfigValidator = (*Config)(nil)
)

// Config defines configuration for jaeger_storage_exporter.
type Config struct {
	TraceStorage string `valid:"required" mapstructure:"trace_storage"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
