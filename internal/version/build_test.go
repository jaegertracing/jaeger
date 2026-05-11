// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
