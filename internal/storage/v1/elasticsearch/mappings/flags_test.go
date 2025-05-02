// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionsWithDefaultFlags(t *testing.T) {
	o := Options{}
	c := cobra.Command{}
	o.AddFlags(&c)

	assert.Empty(t, o.Mapping)
	assert.Equal(t, uint(7), o.EsVersion)
	assert.EqualValues(t, 5, o.Shards)
	assert.EqualValues(t, 1, o.Replicas)

	assert.Empty(t, o.IndexPrefix)
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
	assert.Equal(t, int64(5), o.Shards)
	assert.Equal(t, int64(1), o.Replicas)
	assert.Equal(t, "test", o.IndexPrefix)
	assert.Equal(t, "true", o.UseILM)
	assert.Equal(t, "jaeger-test-policy", o.ILMPolicyName)
}
