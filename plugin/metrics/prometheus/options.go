// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prometheus

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/prometheus/config"
)

const (
	suffixServerURL           = ".server-url"
	suffixConnectTimeout      = ".connect-timeout"
	suffixTokenFilePath       = ".token-file"
	suffixOverrideFromContext = ".token-override-from-context"

	suffixMetricNamespace   = ".query.namespace"
	suffixLatencyUnit       = ".query.duration-unit"
	suffixNormalizeCalls    = ".query.normalize-calls"
	suffixNormalizeDuration = ".query.normalize-duration"

	defaultServerURL      = "http://localhost:9090"
	defaultConnectTimeout = 30 * time.Second
	defaultTokenFilePath  = ""

	defaultSupportSpanmetricsConnector = true
	defaultMetricNamespace             = "prometheus"
	defaultLatencyUnit                 = "ms"
	defaultNormalizeCalls              = false
	defaultNormalizeDuration           = false
)

// Options stores the configuration entries for this storage.
type Options struct {
	config.Configuration `mapstructure:",squash"`
}

// NewOptions creates a new Options struct.
func NewOptions() *Options {
	defaultConfig := config.Configuration{
		ServerURL:      defaultServerURL,
		ConnectTimeout: defaultConnectTimeout,

		MetricNamespace:   defaultMetricNamespace,
		LatencyUnit:       defaultLatencyUnit,
		NormalizeCalls:    defaultNormalizeCalls,
		NormalizeDuration: defaultNormalizeCalls,
	}

	return &Options{
		Configuration: defaultConfig,
	}
}

// AddFlags from this storage to the CLI.
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(defaultMetricNamespace+suffixServerURL, defaultServerURL,
		"The Prometheus server's URL, must include the protocol scheme e.g. http://localhost:9090")
	flagSet.Duration(defaultMetricNamespace+suffixConnectTimeout, defaultConnectTimeout,
		"The period to wait for a connection to Prometheus when executing queries.")
	flagSet.String(defaultMetricNamespace+suffixTokenFilePath, defaultTokenFilePath,
		"The path to a file containing the bearer token which will be included when executing queries against the Prometheus API.")
	flagSet.Bool(defaultMetricNamespace+suffixOverrideFromContext, true,
		"Whether the bearer token should be overridden from context (incoming request)")
	flagSet.String(defaultMetricNamespace+suffixMetricNamespace, defaultMetricNamespace,
		`The metric namespace that is prefixed to the metric name. A '.' separator will be added between `+
			`the namespace and the metric name.`)
	flagSet.String(defaultMetricNamespace+suffixLatencyUnit, defaultLatencyUnit,
		`The units used for the "latency" histogram. It can be either "ms" or "s" and should be consistent with the `+
			`histogram unit value set in the spanmetrics connector (see: `+
			`https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/connector/spanmetricsconnector#configurations). `+
			`This also helps jaeger-query determine the metric name when querying for "latency" metrics.`)
	flagSet.Bool(defaultMetricNamespace+suffixNormalizeCalls, defaultNormalizeCalls,
		`Whether to normalize the "calls" metric name according to `+
			`https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/pkg/translator/prometheus/README.md. `+
			`For example: `+
			`"calls" (not normalized) -> "calls_total" (normalized), `)
	flagSet.Bool(defaultMetricNamespace+suffixNormalizeDuration, defaultNormalizeDuration,
		`Whether to normalize the "duration" metric name according to `+
			`https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/pkg/translator/prometheus/README.md. `+
			`For example: `+
			`"duration_bucket" (not normalized) -> "duration_milliseconds_bucket (normalized)"`)

	opt.getTLSFlagsConfig().AddFlags(flagSet)
}

// InitFromViper initializes the options struct with values from Viper.
func (opt *Options) InitFromViper(v *viper.Viper) error {
	opt.ServerURL = stripWhiteSpace(v.GetString(defaultMetricNamespace + suffixServerURL))
	opt.ConnectTimeout = v.GetDuration(defaultMetricNamespace + suffixConnectTimeout)
	opt.TokenFilePath = v.GetString(defaultMetricNamespace + suffixTokenFilePath)

	opt.MetricNamespace = v.GetString(defaultMetricNamespace + suffixMetricNamespace)
	opt.LatencyUnit = v.GetString(defaultMetricNamespace + suffixLatencyUnit)
	opt.NormalizeCalls = v.GetBool(defaultMetricNamespace + suffixNormalizeCalls)
	opt.NormalizeDuration = v.GetBool(defaultMetricNamespace + suffixNormalizeDuration)
	opt.TokenOverrideFromContext = v.GetBool(defaultMetricNamespace + suffixOverrideFromContext)

	isValidUnit := map[string]bool{"ms": true, "s": true}
	if _, ok := isValidUnit[opt.LatencyUnit]; !ok {
		return fmt.Errorf(`duration-unit must be one of "ms" or "s", not %q`, opt.LatencyUnit)
	}

	var err error
	opt.TLS, err = opt.getTLSFlagsConfig().InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to process Prometheus TLS options: %w", err)
	}
	return nil
}

func (*Options) getTLSFlagsConfig() tlscfg.ClientFlagsConfig {
	return tlscfg.ClientFlagsConfig{
		Prefix: defaultMetricNamespace,
	}
}

// stripWhiteSpace removes all whitespace characters from a string.
func stripWhiteSpace(str string) string {
	return strings.ReplaceAll(str, " ", "")
}
