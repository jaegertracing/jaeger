// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
	prefix = "prometheus"

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

	// the default configuration here matches the default namespace in the span metrics connector
	defaultMetricNamespace   = "traces_span_metrics"
	defaultLatencyUnit       = "ms"
	defaultNormalizeCalls    = false
	defaultNormalizeDuration = false
)

// Options stores the configuration entries for this storage.
type Options struct {
	config.Configuration `mapstructure:",squash"`
}

var tlsFlagsCfg = tlscfg.ClientFlagsConfig{Prefix: prefix}

func DefaultConfig() config.Configuration {
	return config.Configuration{
		ServerURL:      defaultServerURL,
		ConnectTimeout: defaultConnectTimeout,

		MetricNamespace:   defaultMetricNamespace,
		LatencyUnit:       defaultLatencyUnit,
		NormalizeCalls:    defaultNormalizeCalls,
		NormalizeDuration: defaultNormalizeCalls,
	}
}

// NewOptions creates a new Options struct.
func NewOptions() *Options {
	return &Options{
		Configuration: DefaultConfig(),
	}
}

// AddFlags from this storage to the CLI.
func (*Options) AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(prefix+suffixServerURL, defaultServerURL,
		"The Prometheus server's URL, must include the protocol scheme e.g. http://localhost:9090")
	flagSet.Duration(prefix+suffixConnectTimeout, defaultConnectTimeout,
		"The period to wait for a connection to Prometheus when executing queries.")
	flagSet.String(prefix+suffixTokenFilePath, defaultTokenFilePath,
		"The path to a file containing the bearer token which will be included when executing queries against the Prometheus API.")
	flagSet.Bool(prefix+suffixOverrideFromContext, true,
		"Whether the bearer token should be overridden from context (incoming request)")
	flagSet.String(prefix+suffixMetricNamespace, defaultMetricNamespace,
		`The metric namespace that is prefixed to the metric name. A '.' separator will be added between `+
			`the namespace and the metric name.`)
	flagSet.String(prefix+suffixLatencyUnit, defaultLatencyUnit,
		`The units used for the "latency" histogram. It can be either "ms" or "s" and should be consistent with the `+
			`histogram unit value set in the spanmetrics connector (see: `+
			`https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/connector/spanmetricsconnector#configurations). `+
			`This also helps jaeger-query determine the metric name when querying for "latency" metrics.`)
	flagSet.Bool(prefix+suffixNormalizeCalls, defaultNormalizeCalls,
		`Whether to normalize the "calls" metric name according to `+
			`https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/pkg/translator/prometheus/README.md. `+
			`For example: `+
			`"calls" (not normalized) -> "calls_total" (normalized), `)
	flagSet.Bool(prefix+suffixNormalizeDuration, defaultNormalizeDuration,
		`Whether to normalize the "duration" metric name according to `+
			`https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/pkg/translator/prometheus/README.md. `+
			`For example: `+
			`"duration_bucket" (not normalized) -> "duration_milliseconds_bucket (normalized)"`)

	tlsFlagsCfg.AddFlags(flagSet)
}

// InitFromViper initializes the options struct with values from Viper.
func (opt *Options) InitFromViper(v *viper.Viper) error {
	opt.ServerURL = stripWhiteSpace(v.GetString(prefix + suffixServerURL))
	opt.ConnectTimeout = v.GetDuration(prefix + suffixConnectTimeout)
	opt.TokenFilePath = v.GetString(prefix + suffixTokenFilePath)

	opt.MetricNamespace = v.GetString(prefix + suffixMetricNamespace)
	opt.LatencyUnit = v.GetString(prefix + suffixLatencyUnit)
	opt.NormalizeCalls = v.GetBool(prefix + suffixNormalizeCalls)
	opt.NormalizeDuration = v.GetBool(prefix + suffixNormalizeDuration)
	opt.TokenOverrideFromContext = v.GetBool(prefix + suffixOverrideFromContext)

	isValidUnit := map[string]bool{"ms": true, "s": true}
	if _, ok := isValidUnit[opt.LatencyUnit]; !ok {
		return fmt.Errorf(`duration-unit must be one of "ms" or "s", not %q`, opt.LatencyUnit)
	}

	var err error
	tlsCfg, err := tlsFlagsCfg.InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to process Prometheus TLS options: %w", err)
	}
	opt.TLS = tlsCfg
	return nil
}

// stripWhiteSpace removes all whitespace characters from a string.
func stripWhiteSpace(str string) string {
	return strings.ReplaceAll(str, " ", "")
}
