// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/integration"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

// The backward-compatibility workflow checks out an earlier revision alongside the pull request,
// builds ./cmd/jaeger from it, and points these two variables at the resulting binary and at that
// revision's configuration files. JAEGER_OLD_VERSION names that revision for MinWriterVersion
// skips: a release tag such as v2.23.0, or "main".
const (
	oldBinaryEnvVar    = "JAEGER_OLD_BINARY"
	oldConfigDirEnvVar = "JAEGER_OLD_CONFIG_DIR"
	oldVersionEnvVar   = "JAEGER_OLD_VERSION"
)

// oldVersionMain is the JAEGER_OLD_VERSION value the workflow sets when the earlier revision is
// main rather than a release tag.
const oldVersionMain = "main"

// compatScenario defines an explicit upgrade scenario for backward-compatibility testing.
type compatScenario struct {
	Name     string
	OldGates []string // Feature gates for the earlier binary (writer)
	NewGates []string // Feature gates for the current binary revision (reader)
	// OldEnv and NewEnv are the environments of the earlier and the current binary, for the
	// settings their configuration files read from the environment. The earlier binary runs its
	// own revision's configuration file, which ignores a variable it does not read, so OldEnv
	// states what the scenario needs of the writer and only takes effect once that revision's
	// file reads it. The Elasticsearch and OpenSearch scenarios turn the numeric attribute
	// mapping off for the writer either way: a revision from before the setting existed never
	// installs it, and the ordinary e2e suites already cover a reader over indices created with
	// it, so what the scenarios tell apart is the new binary's setting.
	OldEnv       map[string]string
	NewEnv       map[string]string
	Capabilities capabilities.Capabilities
	// MinWriterVersion is the earliest release tag whose binary can write this scenario's
	// storage format (for example "v2.24.0"). When JAEGER_OLD_VERSION is an older release, the
	// scenario is skipped. The main baseline is never skipped for this reason: it can write a
	// format as soon as the option merges. Empty means every baseline can write the format.
	MinWriterVersion string
	// WriteConfigFile, when set, is the config file the earlier binary runs with instead of that
	// revision's copy of suite.ConfigFile. Use it when the scenario's config is generated in the
	// test and does not exist in the earlier checkout (for example ClickHouse attribute_layout:
	// map).
	WriteConfigFile string
}

// runBackwardCompatibilityTests writes the corpus with a Jaeger built from an earlier revision,
// stops it, and reads the corpus back with the Jaeger built from this checkout. That is what
// catches a change to the storage schema or to the span encoding that leaves this revision unable
// to read what the earlier one wrote.
//
// Nothing here specifies which backend it is running against, so a backend joins the battery by
// calling this from its own test file and stays out of it by not calling it.
func runBackwardCompatibilityTests(t *testing.T, storage string, suite E2EStorageIntegration, scenarios ...compatScenario) {
	require.NotEmpty(t, scenarios, "at least one backward compatibility scenario must be provided")

	oldBinary := os.Getenv(oldBinaryEnvVar)
	oldConfigDir := os.Getenv(oldConfigDirEnvVar)
	if oldBinary == "" || oldConfigDir == "" {
		t.Skipf("This test requires %s and %s to point at a Jaeger built from an earlier revision",
			oldBinaryEnvVar, oldConfigDirEnvVar)
	}
	oldVersion := os.Getenv(oldVersionEnvVar)

	for i := range scenarios {
		scenario := &scenarios[i]
		t.Run(scenario.Name, func(t *testing.T) {
			skip, reason := mustSkipForWriterVersion(t, oldVersion, scenario.MinWriterVersion)
			if skip {
				t.Skip(reason)
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
				// than to ./cmd/jaeger. A scenario whose config is generated in the test passes it
				// through WriteConfigFile instead, so the write phase does not look for that file in
				// the earlier checkout.
				writePhase.ConfigFile = writeConfigFile(suite.ConfigFile, oldConfigDir, scenario.WriteConfigFile)
				writePhase.FeatureGates = scenario.OldGates
				writePhase.EnvVarOverrides = scenario.OldEnv
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
				readPhase.EnvVarOverrides = scenario.NewEnv
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

// writeConfigFile chooses the config the earlier binary runs with. A generated WriteConfigFile
// wins; otherwise the earlier checkout's copy of suiteConfigFile is used.
func writeConfigFile(suiteConfigFile, oldConfigDir, writeConfigFile string) string {
	if writeConfigFile != "" {
		return writeConfigFile
	}
	return filepath.Join(oldConfigDir, filepath.Base(suiteConfigFile))
}

// mustSkipForWriterVersion reports whether the scenario should skip because the earlier
// revision predates minWriterVersion. An empty minWriterVersion never skips. The main baseline
// never skips: it can write a format as soon as the option merges. A scenario that names a
// minimum and runs without JAEGER_OLD_VERSION set is skipped, so a forgotten workflow variable
// does not start a writer that cannot write the format. An unparseable version fails the test.
func mustSkipForWriterVersion(t *testing.T, writerVersion, minWriterVersion string) (bool, string) {
	t.Helper()
	if minWriterVersion == "" {
		return false, ""
	}
	if writerVersion == "" {
		return true, fmt.Sprintf("scenario requires %s (minimum writer %s)", oldVersionEnvVar, minWriterVersion)
	}
	if writerVersion == oldVersionMain {
		return false, ""
	}
	cmp, err := compareReleaseVersions(writerVersion, minWriterVersion)
	require.NoError(t, err, "compare writer version %q to minimum %q", writerVersion, minWriterVersion)
	if cmp < 0 {
		return true, fmt.Sprintf("earlier revision %s predates minimum writer version %s", writerVersion, minWriterVersion)
	}
	return false, ""
}

// compareReleaseVersions compares two vMAJOR.MINOR.PATCH release tags. It returns -1 if a < b,
// 0 if a == b, and +1 if a > b.
func compareReleaseVersions(a, b string) (int, error) {
	av, err := parseReleaseVersion(a)
	if err != nil {
		return 0, fmt.Errorf("left version: %w", err)
	}
	bv, err := parseReleaseVersion(b)
	if err != nil {
		return 0, fmt.Errorf("right version: %w", err)
	}
	for i := range av {
		if av[i] < bv[i] {
			return -1, nil
		}
		if av[i] > bv[i] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseReleaseVersion(v string) ([3]int, error) {
	var out [3]int
	if !strings.HasPrefix(v, "v") {
		return out, fmt.Errorf("%q is not a vMAJOR.MINOR.PATCH tag", v)
	}
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("%q is not a vMAJOR.MINOR.PATCH tag", v)
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, fmt.Errorf("%q is not a vMAJOR.MINOR.PATCH tag", v)
		}
		out[i] = n
	}
	return out, nil
}
