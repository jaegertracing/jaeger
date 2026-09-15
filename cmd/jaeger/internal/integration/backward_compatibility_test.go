// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

// The backward-compatibility workflow checks out an earlier revision alongside the pull request,
// builds ./cmd/jaeger from it, and points these two variables at the resulting binary and at that
// revision's configuration files.
const (
	oldBinaryEnvVar    = "JAEGER_OLD_BINARY"
	oldConfigDirEnvVar = "JAEGER_OLD_CONFIG_DIR"
)

// compatScenario defines an explicit upgrade scenario for backward-compatibility testing.
type compatScenario struct {
	Name         string
	OldGates     []string // Feature gates for the earlier binary (writer)
	NewGates     []string // Feature gates for the current binary revision (reader)
	Capabilities capabilities.Capabilities
}

// runBackwardCompatibilityTests writes the corpus with a Jaeger built from an earlier revision,
// stops it, and reads the corpus back with the Jaeger built from this checkout. That is what
// catches a change to the storage schema or to the span encoding that leaves this revision unable
// to read what the earlier one wrote.
//
// scenarios is the list of explicit upgrade paths to test. Each scenario defines what feature
// gates the writer and reader run with and what capabilities that combination yields.
func runBackwardCompatibilityTests(t *testing.T, storage string, suite E2EStorageIntegration, scenarios ...compatScenario) {
	require.NotEmpty(t, scenarios, "at least one backward compatibility scenario must be provided")

	oldBinary := os.Getenv(oldBinaryEnvVar)
	oldConfigDir := os.Getenv(oldConfigDirEnvVar)
	if oldBinary == "" || oldConfigDir == "" {
		t.Skipf("This test requires %s and %s to point at a Jaeger built from an earlier revision",
			oldBinaryEnvVar, oldConfigDirEnvVar)
	}

	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			for _, gate := range scenario.OldGates {
				supported, err := probeGate(oldBinary, gate)
				require.NoError(t, err, "failed to probe feature gate %q on earlier binary %s", gate, oldBinary)
				if !supported {
					t.Skipf("earlier binary (%s) does not recognize feature gate %q", binaryVersion(oldBinary), gate)
				}
			}

			// Both phases share one corpus for this scenario, so that the reader compares against
			// the timestamps the writer wrote: loading a fixture moves its dates to a recent day,
			// and a corpus built twice would move them twice.
			corpus := integration.BuildCorpus(t, suite.Fixtures, scenario.Capabilities)

			// Neither phase purges between them, because the read phase asserts against what the write
			// phase left behind. The read phase purges once it is done.
			scenarioSuite := suite
			scenarioSuite.CleanUp = func(*testing.T) {}
			scenarioSuite.Corpus = corpus

			// Each phase works on its own copy, because e2eInitialize records the process it started and
			// the reader and writer it opened against that process.
			t.Run("WritePhase", func(t *testing.T) {
				writePhase := scenarioSuite
				writePhase.BinaryName = "jaeger-old"
				writePhase.BinaryPath = oldBinary
				// The earlier binary runs that revision's copy of the same configuration file, because a
				// file written for this revision may name settings it does not understand. It also runs
				// without the storage cleaner, which belongs to the jaeger-e2e harness binary rather
				// than to ./cmd/jaeger.
				writePhase.ConfigFile = filepath.Join(oldConfigDir, filepath.Base(suite.ConfigFile))
				writePhase.FeatureGates = scenario.OldGates
				writePhase.Capabilities = scenario.Capabilities
				writePhase.SkipStorageCleaner = true
				writePhase.e2eInitialize(t, storage)
				writePhase.WriteCorpus(t)
			})
			// The earlier binary was stopped by that subtest's cleanup, so what the read phase finds is
			// whatever survived its shutdown.
			t.Run("ReadPhase", func(t *testing.T) {
				readPhase := scenarioSuite
				readPhase.BinaryName = "jaeger-new"
				readPhase.FeatureGates = scenario.NewGates
				readPhase.Capabilities = scenario.Capabilities
				readPhase.e2eInitialize(t, storage)
				// Registered after e2eInitialize so that it runs before the binary is stopped: cleanups run
				// in reverse order of registration, and the purge needs the binary's cleaner endpoint.
				t.Cleanup(func() { purge(t) })
				readPhase.AssertCorpus(t)
			})
		})
	}
}

// resolveBinaryPath resolves relative binary paths to absolute paths so commands like
// `featuregate` and `version` can be executed regardless of the working directory.
func resolveBinaryPath(binaryPath string) string {
	if filepath.IsAbs(binaryPath) {
		return binaryPath
	}
	if abs, err := filepath.Abs(filepath.Join("..", "..", "..", "..", binaryPath)); err == nil {
		return abs
	}
	return binaryPath
}

// probeGate asks the binary whether it recognizes the feature gate through the featuregate subcommand.
// Returns true if recognized (exit code 0), false if unrecognized (non-zero exit code), or an error
// if the binary could not be executed.
func probeGate(binaryPath, gate string) (bool, error) {
	cmd := exec.Command(resolveBinaryPath(binaryPath), "featuregate", gate)
	cmd.Dir = "../../../.."
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() != 0 {
		return false, nil
	}
	return false, err
}

// binaryVersion returns the git version or commit of the binary, or its basename if unavailable.
func binaryVersion(binaryPath string) string {
	cmd := exec.Command(resolveBinaryPath(binaryPath), "version")
	cmd.Dir = "../../../.."
	out, err := cmd.Output()
	if err == nil {
		var v struct {
			GitVersion string `json:"gitVersion"`
			GitCommit  string `json:"gitCommit"`
		}
		if json.Unmarshal(out, &v) == nil {
			if v.GitVersion != "" {
				return v.GitVersion
			}
			if v.GitCommit != "" {
				return v.GitCommit
			}
		}
	}
	return filepath.Base(binaryPath)
}
