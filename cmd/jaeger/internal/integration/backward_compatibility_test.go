// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
)

// The backward-compatibility workflow checks out an earlier revision alongside the pull request,
// builds ./cmd/jaeger from it, and points these two variables at the resulting binary and at that
// revision's configuration files.
const (
	oldBinaryEnvVar    = "JAEGER_OLD_BINARY"
	oldConfigDirEnvVar = "JAEGER_OLD_CONFIG_DIR"
)

// runBackwardCompatibilityTests writes the corpus with a Jaeger built from an earlier revision,
// stops it, and reads the corpus back with the Jaeger built from this checkout. That is what
// catches a change to the storage schema or to the span encoding that leaves this revision unable
// to read what the earlier one wrote.
//
// suite is the read phase, and a backend describes it exactly as it describes its ordinary e2e
// suite: its own configuration file, fixtures, capabilities and feature gates. Nothing here knows
// which backend it is running against, so a backend joins the battery by calling this from its own
// test file and stays out of it by not calling it.
func runBackwardCompatibilityTests(t *testing.T, storage string, suite E2EStorageIntegration) {
	oldBinary := os.Getenv(oldBinaryEnvVar)
	oldConfigDir := os.Getenv(oldConfigDirEnvVar)
	if oldBinary == "" || oldConfigDir == "" {
		t.Skipf("This test requires %s and %s to point at a Jaeger built from an earlier revision",
			oldBinaryEnvVar, oldConfigDirEnvVar)
	}

	// Both phases share one corpus, so that the reader compares against the timestamps the writer
	// wrote: loading a fixture moves its dates to a recent day, and a corpus built twice would
	// move them twice.
	suite.Corpus = integration.BuildCorpus(t, suite.Fixtures, suite.Capabilities)
	// Neither phase purges between them, because the read phase asserts against what the write
	// phase left behind. The read phase purges once it is done.
	suite.CleanUp = func(*testing.T) {}

	// Each phase works on its own copy, because e2eInitialize records the process it started and
	// the reader and writer it opened against that process.
	t.Run("WritePhase", func(t *testing.T) {
		writePhase := suite
		writePhase.BinaryName = "jaeger-old"
		writePhase.BinaryPath = oldBinary
		// The earlier binary runs that revision's copy of the same configuration file, because a
		// file written for this revision may name settings it does not understand, and it runs no
		// feature gates, because a gate this revision defines may not exist there either. It also
		// runs without the storage cleaner, which belongs to the jaeger-e2e harness binary rather
		// than to ./cmd/jaeger.
		writePhase.ConfigFile = filepath.Join(oldConfigDir, filepath.Base(suite.ConfigFile))
		writePhase.FeatureGates = nil
		writePhase.SkipStorageCleaner = true
		writePhase.e2eInitialize(t, storage)
		writePhase.WriteCorpus(t)
	})
	// The earlier binary was stopped by that subtest's cleanup, so what the read phase finds is
	// whatever survived its shutdown.
	t.Run("ReadPhase", func(t *testing.T) {
		readPhase := suite
		readPhase.BinaryName = "jaeger-new"
		readPhase.e2eInitialize(t, storage)
		// Registered after e2eInitialize so that it runs before the binary is stopped: cleanups run
		// in reverse order of registration, and the purge needs the binary's cleaner endpoint.
		t.Cleanup(func() { purge(t) })
		readPhase.AssertCorpus(t)
	})
}
