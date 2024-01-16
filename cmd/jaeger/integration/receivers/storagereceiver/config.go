// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagereceiver

import (
	"github.com/asaskevich/govalidator"

	grpcCfg "github.com/jaegertracing/jaeger/plugin/storage/grpc/config"
)

type Config struct {
	GRPC grpcCfg.Configuration `mapstructure:"grpc-plugin"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
