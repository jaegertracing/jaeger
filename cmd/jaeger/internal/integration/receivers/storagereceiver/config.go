// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagereceiver

import (
	"time"

	"github.com/asaskevich/govalidator"
)

type Config struct {
	TraceStorage string        `valid:"required" mapstructure:"trace_storage"`
	PullInterval time.Duration `mapstructure:"pull_interval"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
