// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestKafkaStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageKafka)

	tests := []struct {
		encoding string
		skip     string
	}{
		{encoding: "otlp_proto"},
		{encoding: "otlp_json"},
		{encoding: "jaeger_proto"},
		{encoding: "jaeger_json"},
	}

	for _, test := range tests {
		t.Run(test.encoding, func(t *testing.T) {
			if test.skip != "" {
				t.Skip(test.skip)
			}
			uniqueTopic := fmt.Sprintf("jaeger-spans-%d", time.Now().UnixNano())
			t.Logf("Using unique Kafka topic: %s", uniqueTopic)

			// Unlike the other storage tests where "collector" has access to the storage,
			// here we have two distinct binaries, collector and ingester, and only the ingester
			// has access to the storage and allows the test to query it.
			// We reuse E2EStorageIntegration struct to manage lifecycle of the collector,
			// but the tests are run against the ingester.
			envVarOverrides := map[string]string{
				"KAFKA_TOPIC":    uniqueTopic,
				"KAFKA_ENCODING": test.encoding,
			}

			collector := &E2EStorageIntegration{
				BinaryName:         "jaeger-v2-collector",
				ConfigFile:         "../../config-kafka-collector.yaml",
				SkipStorageCleaner: true,
				EnvVarOverrides:    envVarOverrides,
			}
			collector.e2eInitialize(t, "kafka")
			t.Log("Collector initialized")

			ingester := &E2EStorageIntegration{
				BinaryName:      "jaeger-v2-ingester",
				ConfigFile:      "../../config-kafka-ingester.yaml",
				HealthCheckPort: 14133,
				StorageIntegration: integration.StorageIntegration{
					CleanUp:      purge,
					Capabilities: capabilities.Kafka(),
				},
				EnvVarOverrides: envVarOverrides,
			}
			ingester.e2eInitialize(t, "kafka")
			t.Log("Ingester initialized")

			ingester.RunSpanStoreTests(t)
		})
	}
}

// TestKafkaStorage_SyncElasticsearch exercises the RFC 0007 at-least-once ingest
// path end-to-end: Collector -> Kafka -> Ingester -> Elasticsearch with the
// ingester in synchronous write mode (write_mode: sync, poison_pill_handling: drop,
// a blocking sending queue, and message_marking.after). Unlike TestKafkaStorage,
// which writes to an in-process memory store, this drives the real synchronous ES
// write path over Kafka. It requires both Kafka and Elasticsearch to be running
// (scripts/e2e/kafka.sh starts both).
func TestKafkaStorage_SyncElasticsearch(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageKafka)

	uniqueTopic := fmt.Sprintf("jaeger-spans-sync-es-%d", time.Now().UnixNano())
	t.Logf("Using unique Kafka topic: %s", uniqueTopic)
	envVarOverrides := map[string]string{
		"KAFKA_TOPIC":    uniqueTopic,
		"KAFKA_ENCODING": "otlp_proto",
	}

	collector := &E2EStorageIntegration{
		BinaryName:         "jaeger-v2-collector",
		ConfigFile:         "../../config-kafka-collector.yaml",
		SkipStorageCleaner: true,
		EnvVarOverrides:    envVarOverrides,
	}
	collector.e2eInitialize(t, "kafka")
	t.Log("Collector initialized")

	ingester := &E2EStorageIntegration{
		BinaryName:      "jaeger-v2-ingester",
		ConfigFile:      "../../config-kafka-ingester-sync.yaml",
		FeatureGates:    elasticsearchFilterGates,
		HealthCheckPort: 14133,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Fixtures:     integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			Capabilities: capabilities.Elasticsearch(),
		},
		EnvVarOverrides: envVarOverrides,
	}
	// storage "elasticsearch" makes the storage cleaner purge ES between runs and
	// shrink the service-cache TTL so freshly-written services are queryable.
	ingester.e2eInitialize(t, "elasticsearch")
	t.Log("Ingester initialized")

	ingester.RunSpanStoreTests(t)
}

// TestKafkaStorage_SyncElasticsearch_FaultInjection proves the RFC 0007 M6
// at-least-once property of the synchronous ingester end-to-end: while the
// Elasticsearch write path fails, the Kafka offset does not advance; once the
// backend recovers, every span written during the outage is stored exactly once,
// the offset catches up, and the partition keeps consuming. It runs the same
// Collector -> Kafka -> Ingester -> Elasticsearch pipeline as
// TestKafkaStorage_SyncElasticsearch, with a fault-injecting reverse proxy between
// the ingester and Elasticsearch.
func TestKafkaStorage_SyncElasticsearch_FaultInjection(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageKafka)

	proxy := newESFaultProxy(t, esBaseURL)
	t.Logf("Elasticsearch fault proxy listening on %s", proxy.URL())

	uniqueTopic := fmt.Sprintf("jaeger-spans-sync-es-fault-%d", time.Now().UnixNano())
	t.Logf("Using unique Kafka topic: %s", uniqueTopic)
	envVarOverrides := map[string]string{
		"KAFKA_TOPIC":    uniqueTopic,
		"KAFKA_ENCODING": "otlp_proto",
		"ES_SERVER_URL":  proxy.URL(),
	}

	collector := &E2EStorageIntegration{
		BinaryName:         "jaeger-v2-collector",
		ConfigFile:         "../../config-kafka-collector.yaml",
		SkipStorageCleaner: true,
		EnvVarOverrides:    envVarOverrides,
	}
	collector.e2eInitialize(t, "kafka")
	t.Log("Collector initialized")

	ingester := &E2EStorageIntegration{
		BinaryName:         "jaeger-v2-ingester",
		ConfigFile:         "../../config-kafka-ingester-sync.yaml",
		SkipStorageCleaner: true,
		HealthCheckPort:    14133,
		EnvVarOverrides:    envVarOverrides,
	}
	ingester.e2eInitialize(t, "elasticsearch")
	t.Log("Ingester initialized")

	// The consumer group is the Kafka receiver's default.
	offsets := newKafkaOffsets(t, kafkaBroker(), "otel-collector", uniqueTopic)
	f := &faultInjectionSteps{collector: collector, proxy: proxy, offsets: offsets}

	t.Run("baseline", func(t *testing.T) {
		trace := f.write(t, 0x01)
		f.requireStoredOnce(t, trace)
		f.requireOffsetCaughtUp(t)
	})

	t.Run("backend_down", func(t *testing.T) {
		f.runOutage(t, esFaultReject, 0x02, func(t *testing.T, trace ptrace.Traces) {
			// Nothing reached Elasticsearch, so the trace is absent for the whole outage.
			assert.Zero(t, f.storedSpanCount(t, trace), "no spans must be stored while _bulk is rejected")
		})
	})

	t.Run("ack_lost", func(t *testing.T) {
		f.runOutage(t, esFaultLoseAck, 0x03, func(t *testing.T, trace ptrace.Traces) {
			// The documents were written but the ingester believes they were not.
			// The offset is still held; the recovery below must not duplicate them.
			assert.Equal(t, trace.SpanCount(), f.storedSpanCount(t, trace), "documents are written even though the acknowledgement is lost")
		})
	})

	t.Run("no_stall", func(t *testing.T) {
		trace := f.write(t, 0x04)
		f.requireStoredOnce(t, trace)
		f.requireOffsetCaughtUp(t)
	})
}
