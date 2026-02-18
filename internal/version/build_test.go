// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/metricstest"
)

func TestNewInfoMetrics(t *testing.T) {
	// Initialize the Jaeger-specific testing factory
	factory := metricstest.NewFactory(0)
	defer factory.Stop()

	// Save original values and then Set some dummy values for the global build variables
	origCommitSHA := commitSHA
	origLatestVersion := latestVersion
	origDate := date

	// Schedule the restoration using defer
	defer func() {
		commitSHA = origCommitSHA
		latestVersion = origLatestVersion
		date = origDate
	}()

	commitSHA = "foobar"
	latestVersion = "v1.2.3"
	date = "2026-02-18"

	// Execute the function under test
	info := NewInfoMetrics(factory)

	// Assertions on the returned struct
	require.NotNil(t, info)
	assert.NotNil(t, info.BuildInfo)

	// Verification using the Factory Snapshot
	_, gauges := factory.Snapshot()

	expectedKey := "build_info|build_date=2026-02-18|revision=foobar|version=v1.2.3"

	val, ok := gauges[expectedKey]
	assert.True(t, ok, "Metric not found in snapshot. Found keys: %v", gauges)
	assert.Equal(t, int64(1), val, "The build_info gauge should be set to 1")
}

func TestGet(t *testing.T) {
	commitSHA = "foobar"
	latestVersion = "v1.2.3"
	date = "2024-01-04"

	info := Get()

	assert.Equal(t, commitSHA, info.GitCommit)
	assert.Equal(t, latestVersion, info.GitVersion)
	assert.Equal(t, date, info.BuildDate)
}

func TestString(t *testing.T) {
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
