// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestNewInfoMetrics(t *testing.T) {
	oldCommit := commitSHA
	oldVersion := latestVersion
	oldDate := date

	t.Cleanup(func() {
		commitSHA = oldCommit
		latestVersion = oldVersion
		date = oldDate
	})

	commitSHA = "foobar"
	latestVersion = "v1.2.3"
	date = "2024-01-04"

	infoMetrics := NewInfoMetrics(nil)
	require.NotNil(t, infoMetrics)
}
