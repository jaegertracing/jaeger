// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"time"

	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/config/configtls"
)

// Configuration describes the options to customize the storage behavior.
type Configuration struct {
	ServerURL      string        `valid:"required" mapstructure:"endpoint"`
	ConnectTimeout time.Duration `mapstructure:"connect_timeout"`
	TLS            configtls.ClientConfig

	TokenFilePath            string `mapstructure:"token_file_path"`
	TokenOverrideFromContext bool   `mapstructure:"token_override_from_context"`

	MetricNamespace      string            `mapstructure:"metric_namespace"`
	LatencyUnit          string            `mapstructure:"latency_unit"`
	NormalizeCalls       bool              `mapstructure:"normalize_calls"`
	NormalizeDuration    bool              `mapstructure:"normalize_duration"`
	AdditionalParameters map[string]string `mapstructure:"additional_parameters"`
}

func (c *Configuration) Validate() error {
	_, err := govalidator.ValidateStruct(c)
	return err
}
