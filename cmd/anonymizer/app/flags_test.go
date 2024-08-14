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

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestOptionsWithDefaultFlags(t *testing.T) {
	o := Options{}
	c := cobra.Command{}
	o.AddFlags(&c)

	assert.Equal(t, "localhost:16686", o.QueryGRPCHostPort)
	assert.Equal(t, "/tmp", o.OutputDir)
	assert.False(t, o.HashStandardTags)
	assert.False(t, o.HashCustomTags)
	assert.False(t, o.HashLogs)
	assert.False(t, o.HashProcess)
	assert.Equal(t, -1, o.MaxSpansCount)
}

func TestOptionsWithFlags(t *testing.T) {
	o := Options{}
	c := cobra.Command{}

	o.AddFlags(&c)
	c.ParseFlags([]string{
		"--query-host-port=192.168.1.10:16686",
		"--output-dir=/data",
		"--trace-id=6ef2debb698f2f7c",
		"--hash-standard-tags",
		"--hash-custom-tags",
		"--hash-logs",
		"--hash-process",
		"--max-spans-count=100",
	})

	assert.Equal(t, "192.168.1.10:16686", o.QueryGRPCHostPort)
	assert.Equal(t, "/data", o.OutputDir)
	assert.Equal(t, "6ef2debb698f2f7c", o.TraceID)
	assert.True(t, o.HashStandardTags)
	assert.True(t, o.HashCustomTags)
	assert.True(t, o.HashLogs)
	assert.True(t, o.HashProcess)
	assert.Equal(t, 100, o.MaxSpansCount)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
