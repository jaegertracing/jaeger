// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerclientenv2otel

import (
	"os"

	"go.uber.org/zap"
)

//nolint:gosec // G101 - env var names, not credentials
var envVars = map[string]string{
	"JAEGER_SERVICE_NAME":                           "",
	"JAEGER_AGENT_HOST":                             "OTEL_EXPORTER_JAEGER_AGENT_HOST",
	"JAEGER_AGENT_PORT":                             "OTEL_EXPORTER_JAEGER_AGENT_PORT",
	"JAEGER_ENDPOINT":                               "OTEL_EXPORTER_JAEGER_ENDPOINT",
	"JAEGER_USER":                                   "OTEL_EXPORTER_JAEGER_USER",
	"JAEGER_PASSWORD":                               "OTEL_EXPORTER_JAEGER_PASSWORD",
	"JAEGER_REPORTER_LOG_SPANS":                     "",
	"JAEGER_REPORTER_MAX_QUEUE_SIZE":                "",
	"JAEGER_REPORTER_FLUSH_INTERVAL":                "",
	"JAEGER_REPORTER_ATTEMPT_RECONNECTING_DISABLED": "",
	"JAEGER_REPORTER_ATTEMPT_RECONNECT_INTERVAL":    "",
	"JAEGER_SAMPLER_TYPE":                           "",
	"JAEGER_SAMPLER_PARAM":                          "",
	"JAEGER_SAMPLER_MANAGER_HOST_PORT":              "",
	"JAEGER_SAMPLING_ENDPOINT":                      "",
	"JAEGER_SAMPLER_MAX_OPERATIONS":                 "",
	"JAEGER_SAMPLER_REFRESH_INTERVAL":               "",
	"JAEGER_TAGS":                                   "",
	"JAEGER_TRACEID_128BIT":                         "",
	"JAEGER_DISABLED":                               "",
	"JAEGER_RPC_METRICS":                            "",
}

func MapJaegerToOtelEnvVars(logger *zap.Logger) {
	for jname, otelname := range envVars {
		val := os.Getenv(jname)
		if val == "" {
			continue
		}
		if otelname == "" {
			logger.Sugar().Infof("Ignoring deprecated Jaeger SDK env var %s, as there is no equivalent in OpenTelemetry", jname)
		} else {
			os.Setenv(otelname, val)
			logger.Sugar().Infof("Replacing deprecated Jaeger SDK env var %s with OpenTelemetry env var %s", jname, otelname)
		}
	}
}
