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
	// 1. Initialize the Jaeger-specific testing factory
	// 0 means... we will snapshot it manually.
	factory := metricstest.NewFactory(0)
	defer factory.Stop()

	// 2. Set some dummy values for the global build variables
	commitSHA = "test-sha"
	latestVersion = "v1.2.3"
	date = "2026-02-03"

	// 3. Execute the function under test
	info := NewInfoMetrics(factory)

	// 4. Assertions on the returned struct
	require.NotNil(t, info)
	assert.NotNil(t, info.BuildInfo)

	// 5. Verification using the Factory Snapshot
	_, gauges := factory.Snapshot()

	// The key format for metricstest is "name|tag1=val1|tag2=val2"
	// Note: The order of tags in the string depends on the internal map ordering.
	expectedKey := "build_info|build_date=2026-02-03|revision=test-sha|version=v1.2.3"

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
