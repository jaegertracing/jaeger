// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotestorage

import (
	"github.com/asaskevich/govalidator"

	"github.com/jaegertracing/jaeger/cmd/remote-storage/app"
)

type Config struct {
	app.Options `mapstructure:",squash"`
	Storage     string `mapstructure:"storage" valid:"required"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
