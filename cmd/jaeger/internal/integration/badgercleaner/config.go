// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badgercleaner

import (
	"github.com/asaskevich/govalidator"
)

type Config struct {
	TraceStorage string `valid:"required" mapstructure:"trace_storage"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
