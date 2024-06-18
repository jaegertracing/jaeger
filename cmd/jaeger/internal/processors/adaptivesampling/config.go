// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/component"
)

var _ component.ConfigValidator = (*Config)(nil)

type Config struct {
	// all configuration for the processor is in the remotesampling extension
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
