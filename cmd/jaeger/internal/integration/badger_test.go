// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

// mainJaegerBinary is the @main jaeger binary used during the backward-compatibility write phase.
const mainJaegerBinary = "/tmp/jaeger-at-main/jaeger"

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
	integration.SkipUnlessEnv(t, integration.StorageBadger)
	integration.SkipUnlessBackwardCompatibility(t)

	// Write phase runs the @main binary; read phase runs the current-branch build.
	s := &E2EStorageIntegration{
		BinaryName:          "jaeger-writer",
		BinaryPath:          mainJaegerBinary,
		ConfigFile:          "../../config-badger.yaml",
		PropagateEnvVars:    []string{"BADGER_METRICS_UPDATE_INTERVAL"},
		SkipMetricsScraping: true,
		StorageIntegration: integration.StorageIntegration{
			CleanUp:           func(*testing.T) {},
			Capabilities:      capabilities.Badger(),
			SkipReadingTraces: true,
		},
	}
	s.e2eInitialize(t, "badger")
	purge(t)
	s.RunSpanStoreTests(t)
	// The writer must fully exit before the read phase starts: Badger holds an
	// exclusive lock on the store directory, so the reader can only open it once
	// the writer has released the lock. Stop waits for the process to exit.
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
