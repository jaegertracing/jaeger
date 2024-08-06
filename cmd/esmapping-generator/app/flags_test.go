// Copyright (c) 2020 The Jaeger Authors.
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

package app

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestOptionsWithDefaultFlags(t *testing.T) {
	o := Options{}
	c := cobra.Command{}
	o.AddFlags(&c)

	assert.Equal(t, "", o.Mapping)
	assert.Equal(t, uint(7), o.EsVersion)
	assert.Equal(t, 5, o.Indices.Spans.TemplateOptions.NumShards)
	assert.Equal(t, 5, o.Indices.Services.TemplateOptions.NumShards)
	assert.Equal(t, 5, o.Indices.Dependencies.TemplateOptions.NumShards)
	assert.Equal(t, 5, o.Indices.Sampling.TemplateOptions.NumShards)

	assert.Equal(t, 1, o.Indices.Spans.TemplateOptions.NumReplicas)
	assert.Equal(t, 1, o.Indices.Services.TemplateOptions.NumReplicas)
	assert.Equal(t, 1, o.Indices.Dependencies.TemplateOptions.NumReplicas)
	assert.Equal(t, 1, o.Indices.Sampling.TemplateOptions.NumReplicas)
	assert.Equal(t, "", o.IndexPrefix)
	assert.Equal(t, "false", o.UseILM)
	assert.Equal(t, "jaeger-ilm-policy", o.ILMPolicyName)
}

func TestOptionsWithFlags(t *testing.T) {
	o := Options{}
	c := cobra.Command{}

	o.AddFlags(&c)
	err := c.ParseFlags([]string{
		"--mapping=jaeger-span",
		"--es-version=7",
		"--shards=5",
		"--replicas=1",
		"--index-prefix=test",
		"--use-ilm=true",
		"--ilm-policy-name=jaeger-test-policy",
	})
	require.NoError(t, err)
	assert.Equal(t, "jaeger-span", o.Mapping)
	assert.Equal(t, uint(7), o.EsVersion)
	assert.Equal(t, 5, o.Indices.Spans.TemplateOptions.NumShards)
	assert.Equal(t, 5, o.Indices.Services.TemplateOptions.NumShards)
	assert.Equal(t, 5, o.Indices.Dependencies.TemplateOptions.NumShards)
	assert.Equal(t, 5, o.Indices.Sampling.TemplateOptions.NumShards)

	assert.Equal(t, 1, o.Indices.Spans.TemplateOptions.NumReplicas)
	assert.Equal(t, 1, o.Indices.Services.TemplateOptions.NumReplicas)
	assert.Equal(t, 1, o.Indices.Dependencies.TemplateOptions.NumReplicas)
	assert.Equal(t, 1, o.Indices.Sampling.TemplateOptions.NumReplicas)
	assert.Equal(t, "test", o.IndexPrefix)
	assert.Equal(t, "true", o.UseILM)
	assert.Equal(t, "jaeger-test-policy", o.ILMPolicyName)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
