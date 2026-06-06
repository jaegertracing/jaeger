// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package promcfg

import (
	"fmt"
	"time"

	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/config/configtls"
)

// Configuration describes the options to customize the storage behavior.
type Configuration struct {
	ServerURL      string        `valid:"required" mapstructure:"endpoint"`
	ConnectTimeout time.Duration `mapstructure:"connect_timeout"`
	// TLS contains the TLS configuration for the connection to the Prometheus clusters.
	TLS configtls.ClientConfig `mapstructure:"tls"`

	TokenFilePath            string `mapstructure:"token_file_path"`
	TokenOverrideFromContext bool   `mapstructure:"token_override_from_context"`

	MetricNamespace   string `mapstructure:"metric_namespace"`
	LatencyUnit       string `mapstructure:"latency_unit"`
	NormalizeCalls    bool   `mapstructure:"normalize_calls"`
	NormalizeDuration bool   `mapstructure:"normalize_duration"`
	// ExtraQueryParams is used to provide extra parameters to be appended
	// to the URL of queries going out to the metrics backend.
	ExtraQueryParams map[string]string `mapstructure:"extra_query_parameters"`
}

func (c *Configuration) Validate() error {
	if _, err := govalidator.ValidateStruct(c); err != nil {
		return err
	}
	// Empty is allowed because the default ("ms") is applied during unmarshalling.
	if c.LatencyUnit != "" && !IsValidLatencyUnit(c.LatencyUnit) {
		return fmt.Errorf(`latency_unit must be "ms" or "s", not %q`, c.LatencyUnit)
	}
	return nil
}

// IsValidLatencyUnit reports whether u is a latency unit the Prometheus metric
// name builder understands. It is the single source of truth shared by this
// validation and the v1 flag path in prometheus/options.go.
func IsValidLatencyUnit(u string) bool {
	return u == "ms" || u == "s"
}
