// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap/xconfmap"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

var (
	_ component.Config   = (*Config)(nil)
	_ xconfmap.Validator = (*Config)(nil)
)

// Config defines configuration for jaeger_storage_exporter.
type Config struct {
	TraceStorage string                     `mapstructure:"trace_storage" valid:"required"`
	QueueConfig  exporterhelper.QueueConfig `mapstructure:"queue" valid:"optional"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
