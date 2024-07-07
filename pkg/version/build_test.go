// Copyright (c) 2024 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package version

import (
	"fmt"
	"testing"
	//"time"

	"github.com/stretchr/testify/assert"
	//"github.com/stretchr/testify/mock"

	"github.com/jaegertracing/jaeger/internal/metrics/metricsbuilder"
	"github.com/jaegertracing/jaeger/pkg/metrics"
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

func InitBuilder(b *metricsbuilder.Builder) *metricsbuilder.Builder {
	b.Backend = "prometheus"
	b.HTTPRoute = "/metrics"
	return b
}

func TestNewInfoMetrics(t *testing.T) {
	metricsBuilder := InitBuilder(new(metricsbuilder.Builder))
	metricsFactory, _ := metricsBuilder.CreateMetricsFactory("")
	baseFactory := metricsFactory.Namespace(metrics.NSOptions{Name: "jaeger"})

	info := NewInfoMetrics(baseFactory)
	fmt.Println(info)
	// assert.Equal(t, NewInfoMetrics(factory), "hi")
}
