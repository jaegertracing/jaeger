// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestOpenSearchStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageOpenSearch)
	s := &E2EStorageIntegration{
		ConfigFile:   "../../config-opensearch.yaml",
		FeatureGates: structuredFilterGates,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Fixtures:     integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			Capabilities: capabilities.OpenSearch(),
		},
	}
	s.e2eInitialize(t, "opensearch")
	s.RunSpanStoreTests(t)
}

// TestOpenSearchStorage_TwoPhase writes the corpus with one Jaeger process, stops it, and reads it
// back with a second one. It is what holds the write/read split honest: the write phase performs
// no reads, the read phase performs no writes, and the corpus the two compare against is built
// once, before either process starts.
func TestOpenSearchStorage_TwoPhase(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageOpenSearch)

	fixtures := integration.LoadAndParseQueryTestCases(t, "fixtures/queries_es.json")
	corpus := integration.BuildCorpus(t, fixtures, capabilities.OpenSearch())

	// Neither phase purges between them, because the read phase asserts against what the write
	// phase left behind. The read phase purges once it is done, so the indices do not outlive the
	// test.
	startPhase := func(t *testing.T, binaryName string) *E2EStorageIntegration {
		s := &E2EStorageIntegration{
			ConfigFile:   "../../config-opensearch.yaml",
			BinaryName:   binaryName,
			FeatureGates: structuredFilterGates,
			StorageIntegration: integration.StorageIntegration{
				CleanUp:      func(*testing.T) {},
				Fixtures:     fixtures,
				Capabilities: capabilities.OpenSearch(),
				Corpus:       corpus,
			},
		}
		s.e2eInitialize(t, "opensearch")
		return s
	}

	t.Run("WritePhase", func(t *testing.T) {
		startPhase(t, "jaeger-write-phase").WriteCorpus(t)
	})
	// The write phase's binary was stopped by that subtest's cleanup, so what the read phase finds
	// is whatever survived its shutdown.
	t.Run("ReadPhase", func(t *testing.T) {
		s := startPhase(t, "jaeger-read-phase")
		// Registered after e2eInitialize so that it runs before the binary is stopped: cleanups run
		// in reverse order of registration, and the purge needs the binary's cleaner endpoint.
		t.Cleanup(func() { purge(t) })
		s.AssertCorpus(t)
	})
}

func TestOpenSearchStorage_ManualRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageOpenSearch)
	setupManualRolloverIndices(t, "jaeger-mr")
	runRotationSmokeTest(t, "../../config-opensearch-manual-rollover.yaml", "opensearch", func(t *testing.T) {
		initManualRolloverIndices(t, "jaeger-mr")
	})
}

func TestOpenSearchStorage_AutoRollover(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageOpenSearch)
	setupAutoRolloverIndices(t, "jaeger-ar", "jaeger-test-ilm-policy")
	runRotationSmokeTest(t, "../../config-opensearch-auto-rollover.yaml", "opensearch", func(t *testing.T) {
		initAutoRolloverIndices(t, "jaeger-ar", "jaeger-test-ilm-policy")
	})
}

func TestOpenSearchStorage_DataStream(t *testing.T) {
	t.Skip("data_stream rotation not yet implemented (see RFC 0004 Phase 2)")
	integration.SkipUnlessEnv(t, integration.StorageOpenSearch)
	runRotationSmokeTest(t, "../../config-opensearch-data-stream.yaml", "opensearch", func(*testing.T) {})
}
