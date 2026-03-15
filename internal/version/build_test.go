// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/stretchr/testify/assert"
)

func setupBuildForTest(t *testing.T, commit, version, buildDate string) {
	oldCommitSHA := commitSHA
	oldLatestVersion := latestVersion
	oldDate := date

	t.Cleanup(func() {
		commitSHA = oldCommitSHA
		latestVersion = oldLatestVersion
		date = oldDate
	})

	commitSHA = commit
	latestVersion = version
	date = buildDate
}

func TestGet(t *testing.T) {
	setupBuildForTest(t, "foobar", "v1.2.3", "2024-01-01")
	info := Get()

	assert.Equal(t, commitSHA, info.GitCommit)
	assert.Equal(t, latestVersion, info.GitVersion)
	assert.Equal(t, date, info.BuildDate)
}

func TestString(t *testing.T) {
	setupBuildForTest(t, "foobar", "v1.2.3", "2024-01-01")
	test := Info{
		GitCommit:  commitSHA,
		GitVersion: latestVersion,
		BuildDate:  date,
	}
	expectedOutput := "git-commit=foobar, git-version=v1.2.3, build-date=2024-01-04"
	assert.Equal(t, expectedOutput, test.String())
}

func TestNewInfoMetrics(t *testing.T) {
	setupBuildForTest(t, "foobar", "v1.2.3", "2024-01-01")
	f := metricstest.NewFactory(0)
	defer f.Stop()

	NewInfoMetrics(f)

	f.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name:  "build_info",
		Value: 1,
		Tags: map[string]string{
			"build_date": "2024-01-04",
			"revision":   "foobar",
			"version":    "v1.2.3",
		},
	})
}
