// Copyright (c) 2017 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"fmt"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

var (
	// commitFromGit is a constant representing the source version that
	// generated this build. It should be set during build via -ldflags.
	commitSHA string
	// versionFromGit is a constant representing the version tag that
	// generated this build. It should be set during build via -ldflags.
	latestVersion string
	// build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
	date string
)

// Info holds build information.
type Info struct {
	GitCommit  string `json:"gitCommit"`
	GitVersion string `json:"gitVersion"`
	BuildDate  string `json:"buildDate"`
}

// InfoMetrics hold a gauge whose tags include build information.
type InfoMetrics struct {
	BuildInfo metrics.Gauge `metric:"build_info"`
}

// Get creates and initialized Info object
func Get() Info {
	return Info{
		GitCommit:  commitSHA,
		GitVersion: latestVersion,
		BuildDate:  date,
	}
}

// NewInfoMetrics returns a InfoMetrics
func NewInfoMetrics(metricsFactory metrics.Factory) *InfoMetrics {
	var info InfoMetrics

	buildTags := map[string]string{
		"revision":   commitSHA,
		"version":    latestVersion,
		"build_date": date,
	}
	metrics.Init(&info, metricsFactory, buildTags)
	info.BuildInfo.Update(1)

	return &info
}

func (i Info) String() string {
	return fmt.Sprintf(
		"git-commit=%s, git-version=%s, build-date=%s",
		i.GitCommit, i.GitVersion, i.BuildDate,
	)
}
