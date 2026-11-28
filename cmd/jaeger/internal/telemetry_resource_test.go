// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"
)

// TestShippedConfigsUseTelemetryResourceAttributes guards the startup warning
// from an inline service.telemetry.resource map. The collector accepts that
// shape, then warns and asks for resource.attributes. all-in-one.yaml is what
// `jaeger` runs when no --config is passed, so a missing resource block there
// is a failure, not a skip.
func TestShippedConfigsUseTelemetryResourceAttributes(t *testing.T) {
	// config-kafka-ingester-dead-letter.yaml references this with no default.
	t.Setenv("DEAD_LETTER_ENDPOINT", "http://127.0.0.1:4318/v1/traces")

	configs, err := filepath.Glob(filepath.Join("..", "config*.yaml"))
	require.NoError(t, err)
	configs = append(configs,
		"all-in-one.yaml",
		filepath.Join("..", "..", "..", "examples", "oci", "config.yaml"),
		filepath.Join("..", "..", "..", "docker-compose", "kafka", "jaeger-ingester-remote-storage.yaml"),
	)

	for _, path := range configs {
		t.Run(path, func(t *testing.T) {
			conf := resolveBatchingConfig(t, path)
			if !conf.IsSet("service::telemetry::resource") {
				if telemetryResourceRequired(path) {
					t.Fatal("service.telemetry.resource.attributes is required")
				}
				return
			}
			sub, err := conf.Sub("service::telemetry::resource")
			require.NoError(t, err)

			var resource otelconftelemetry.ResourceConfig
			require.NoError(t, sub.Unmarshal(&resource))
			require.NoError(t, resource.Validate())
			require.Empty(t, resource.LegacyAttributes)

			var serviceNames []any
			for _, attr := range resource.Attributes {
				if attr.Name == "service.name" {
					serviceNames = append(serviceNames, attr.Value)
				}
			}
			require.Equal(t, []any{expectedTelemetryServiceName(path)}, serviceNames)
		})
	}
}

// telemetryResourceRequired reports whether path is one of the shipped configs
// that sets service.name. The SPM samples never set service.telemetry.resource,
// so they cannot emit the legacy warning; any other file in this test must.
func telemetryResourceRequired(path string) bool {
	switch filepath.Base(path) {
	case "config-spm-clickhouse.yaml", "config-spm-elasticsearch.yaml", "config-spm-opensearch.yaml":
		return false
	default:
		return true
	}
}

func expectedTelemetryServiceName(path string) string {
	switch filepath.Base(path) {
	case "config-query.yaml":
		return "jaeger-query"
	case "config-kafka-collector.yaml":
		return "jaeger_collector"
	case "config-kafka-ingester-dead-letter.yaml", "config-kafka-ingester-sync.yaml", "jaeger-ingester-remote-storage.yaml":
		return "jaeger_ingester"
	default:
		return "jaeger"
	}
}
