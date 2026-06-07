// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package promcfg

import (
	"fmt"
	"regexp"
	"time"

	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

// dimensionNameRegex limits dimension names to OTel-style attribute identifiers.
// After dot→underscore conversion, the result also satisfies the Prometheus
// label-name regex `^[A-Za-z_][A-Za-z0-9_]*$`.
var dimensionNameRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)

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
	// ExtraDimensions declares a fixed, pre-configured set of additional
	// Prometheus labels that the SPM API will accept as filters and inject
	// into PromQL queries. Names must match dimensions declared in the
	// spanmetrics connector config (so the labels exist on the stored time
	// series). Free-form/arbitrary tag filtering is intentionally not
	// supported because PromQL requires labels to be declared up front.
	ExtraDimensions []metricstore.Dimension `mapstructure:"extra_dimensions" valid:"-"`
}

func (c *Configuration) Validate() error {
	if _, err := govalidator.ValidateStruct(c); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(c.ExtraDimensions))
	for i, d := range c.ExtraDimensions {
		if !dimensionNameRegex.MatchString(d.Name) {
			return fmt.Errorf("extra_dimensions[%d]: invalid name %q (must match %s)",
				i, d.Name, dimensionNameRegex.String())
		}
		if _, dup := seen[d.Name]; dup {
			return fmt.Errorf("extra_dimensions[%d]: duplicate name %q", i, d.Name)
		}
		seen[d.Name] = struct{}{}
	}
	return nil
}
