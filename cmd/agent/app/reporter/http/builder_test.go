// Copyright (c) 2017 Uber Technologies, Inc.
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

package http

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"
)

var yamlConfig = `
collectorEndpoint: http://mock.collector.endpoint.com
requestTimeout: 10s
`

const mockCollectorEndpoint = "http://mock.collector.endpoint.com"

func TestBuilderFromConfig(t *testing.T) {
	b := Builder{}
	err := yaml.Unmarshal([]byte(yamlConfig), &b)
	require.NoError(t, err)

	assert.Equal(t, mockCollectorEndpoint, b.CollectorEndpoint)
	assert.Equal(t, time.Second*10, b.RequestTimeout)
	r, err := b.CreateReporter()
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, mockCollectorEndpoint, r.CollectorEndpoint())

}

func TestBuilderWithCollectorEndpoint(t *testing.T) {
	cfg := &Builder{}
	r, err := cfg.CreateReporter()
	assert.Nil(t, r)
	assert.EqualError(t, err, "Invalid Parameter CollectorEndpoint, Non-empty onme required")

	cfg = &Builder{}
	cfg.WithCollectorEndpoint(mockCollectorEndpoint)
	reporter, err := cfg.CreateReporter()
	assert.NoError(t, err)
	assert.NotNil(t, reporter)
	assert.NotNil(t, reporter.client)
}
