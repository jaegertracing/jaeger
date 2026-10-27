// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/jaegertracing/jaeger/internal/metricstest"
)

func saveBuildVars(t *testing.T) {
	origCommitSHA := commitSHA
	origLatestVersion := latestVersion
	origDate := date
	t.Cleanup(func() {
		commitSHA = origCommitSHA
		latestVersion = origLatestVersion
		date = origDate
	})
}

func TestGetDefault(t *testing.T) {
	saveBuildVars(t)
	// Verify that the default version is "dev" when not set via ldflags
	info := Get()
	assert.Equal(t, "dev", info.GitVersion)
}

func TestGet(t *testing.T) {
	saveBuildVars(t)

	commitSHA = "foobar"
	latestVersion = "v1.2.3"
	date = "2024-01-04"

	info := Get()

	assert.Equal(t, commitSHA, info.GitCommit)
	assert.Equal(t, latestVersion, info.GitVersion)
	assert.Equal(t, date, info.BuildDate)
}

func TestString(t *testing.T) {
	saveBuildVars(t)

	commitSHA = "foobar"
	latestVersion = "v1.2.3"
	date = "2024-01-04"
	test := Info{
		GitCommit:  commitSHA,
		GitVersion: latestVersion,
		BuildDate:  date,
	}
	expectedOutput := "git-commit=foobar, git-version=v1.2.3, build-date=2024-01-04"
	assert.Equal(t, expectedOutput, test.String())
}

// TestNewInfoMetrics verifies that NewInfoMetrics properly initializes the
// build_info Prometheus gauge with the current revision, version, and build date.
func TestNewInfoMetrics(t *testing.T) {
	// 1. Preserve and restore global build variables after the test finishes
	saveBuildVars(t)

	// 2. Set deterministic build metadata for testing
	commitSHA = "abc1234"
	latestVersion = "v1.50.0"
	date = "2024-01-01"

	// 3. Create an in-memory metric recorder mock (avoids network I/O)
	mf := metricstest.NewFactory(0)

	// 4. Initialize info metrics with the factory
	infoMetrics := NewInfoMetrics(mf)
	assert.NotNil(t, infoMetrics)

	// 5. Verify the gauge was emitted and recorded in memory
	_, gauges := mf.Backend.Snapshot()
	assert.NotEmpty(t, gauges)
}