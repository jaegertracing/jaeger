// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagecleaner

import (
	"github.com/asaskevich/govalidator"
)

type Config struct {
	TraceStorage string `valid:"required" mapstructure:"trace_storage"`
	Port         string `mapstructure:"port"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
