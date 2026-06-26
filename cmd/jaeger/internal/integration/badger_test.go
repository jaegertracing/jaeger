// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestBadgerStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, integration.StorageBadger)

	s := &E2EStorageIntegration{
		ConfigFile:       "../../config-badger.yaml",
		PropagateEnvVars: []string{"BADGER_METRICS_UPDATE_INTERVAL"},
		StorageIntegration: integration.StorageIntegration{
			CleanUp:      purge,
			Capabilities: capabilities.Badger(),
		},
	}
	s.e2eInitialize(t, "badger")
	s.RunAll(t)
}

// TestBadgerBackwardCompatibility verifies that traces written by a previous Jaeger
// binary remain readable by the current branch binary, simulating a rolling upgrade
// against a shared Badger store.
func TestBadgerBackwardCompatibility(t *testing.T) {
	if os.Getenv("BACKWARD_COMPATIBILITY") != "true" {
		t.Skip("set BACKWARD_COMPATIBILITY=true to run backward compatibility tests")
	}
	integration.SkipUnlessEnv(t, integration.StorageBadger)

	// The write phase must run a different binary than the read phase, otherwise the
	// test passes without exercising cross-version compatibility at all.
	writerBinary := os.Getenv("JAEGER_BACKWARD_COMPAT_BINARY")
	require.NotEmpty(t, writerBinary, "set JAEGER_BACKWARD_COMPAT_BINARY to the previous-version jaeger binary")

	// GetServices is skipped: testFindTraces accumulates query-fixture service names
	// in the write phase, breaking its strict equality check in the read phase.
	caps := capabilities.Badger().WithSkip("GetServices")

	s := &E2EStorageIntegration{
		BinaryName:          "jaeger-writer",
		BinaryPath:          writerBinary,
		ConfigFile:          "../../config-badger.yaml",
		PropagateEnvVars:    []string{"BADGER_METRICS_UPDATE_INTERVAL"},
		SkipMetricsScraping: true,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:           func(*testing.T) {},
			Capabilities:      caps,
			SkipReadingTraces: true,
		},
	}
	s.e2eInitialize(t, "badger")
	purge(t)
	s.RunSpanStoreTests(t)
	s.binary.Stop(t)

	e := *s
	e.BinaryName = ""
	e.BinaryPath = ""
	// testFindTraces appends to Fixtures, so reset it to avoid double-loading
	// the query fixtures already populated during the write phase.
	e.Fixtures = nil
	e.SkipReadingTraces = false
	e.SkipWritingTraces = true
	e.e2eInitialize(t, "badger")
	e.RunSpanStoreTests(t)
	purge(t)
}
