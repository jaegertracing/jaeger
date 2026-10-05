// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

var (
	_ component.Config  = (*Config)(nil)
	_ confmap.Validator = (*Config)(nil)
)

// Config defines configuration for jaeger_storage_exporter in both of its forms:
// the exporter (declared under `exporters:`) and the connector with a dead-letter
// output (declared under `connectors:`). The connector form additionally requires an
// enabled queue to set wait_for_result; that check lives in its factory because
// the exporter form must keep accepting a queue that acknowledges on enqueue.
type Config struct {
	TraceStorage string                                                   `mapstructure:"trace_storage" valid:"required"`
	QueueConfig  configoptional.Optional[exporterhelper.QueueBatchConfig] `mapstructure:"queue" valid:"optional"`
	RetryConfig  configretry.BackOffConfig                                `mapstructure:"retry_on_failure"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
