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
)

func TestOptionsWithDefaultFlags(t *testing.T) {
	o := Options{}
	c := cobra.Command{}
	o.AddFlags(&c)

	assert.Equal(t, "", o.Mapping)
	assert.Equal(t, int64(7), o.EsVersion)
	assert.Equal(t, int64(5), o.Shards)
	assert.Equal(t, int64(1), o.Replicas)
	assert.Equal(t, "", o.EsPrefix)
	assert.Equal(t, "false", o.UseILM)
}

func TestOptionsWithFlags(t *testing.T) {
	o := Options{}
	c := cobra.Command{}

	o.AddFlags(&c)
	err := c.ParseFlags([]string{
		"--mapping=jaeger-span",
		"--es-version=6",
		"--shards=5",
		"--replicas=1",
		"--es-prefix=test",
		"--use-ilm=true",
	})
	require.NoError(t, err)
	assert.Equal(t, "jaeger-span", o.Mapping)
	assert.Equal(t, int64(6), o.EsVersion)
	assert.Equal(t, int64(5), o.Shards)
	assert.Equal(t, int64(1), o.Replicas)
	assert.Equal(t, "test", o.EsPrefix)
	assert.Equal(t, "true", o.UseILM)
}
