// Copyright (c) 2021 The Jaeger Authors.
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

package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/es/client"
)

var indices = []client.Index{
	{
		Index: "jaeger-span-0001",
		Aliases: map[string]bool{
			"jaeger-span-write": true,
			"jaeger-span-read":  true,
		},
	},
	{
		Index: "jaeger-span-0002",
		Aliases: map[string]bool{
			"jaeger-span-read": true,
		},
	},
	{
		Index: "jaeger-span-0003",
		Aliases: map[string]bool{
			"jaeger-span-read": true,
		},
	},
	{
		Index: "jaeger-span-0004",
		Aliases: map[string]bool{
			"jaeger-span-other": true,
		},
	},
	{
		Index: "jaeger-span-0005",
		Aliases: map[string]bool{
			"custom-alias": true,
		},
	},
	{
		Index: "jaeger-span-0006",
	},
}

func TestByAlias(t *testing.T) {
	filtered := ByAlias(indices, []string{"jaeger-span-read", "jaeger-span-other"})
	expected := []client.Index{
		{
			Index: "jaeger-span-0001",
			Aliases: map[string]bool{
				"jaeger-span-write": true,
				"jaeger-span-read":  true,
			},
		},
		{
			Index: "jaeger-span-0002",
			Aliases: map[string]bool{
				"jaeger-span-read": true,
			},
		},
		{
			Index: "jaeger-span-0003",
			Aliases: map[string]bool{
				"jaeger-span-read": true,
			},
		},
		{
			Index: "jaeger-span-0004",
			Aliases: map[string]bool{
				"jaeger-span-other": true,
			},
		},
	}
	assert.Equal(t, expected, filtered)
}

func TestByAliasExclude(t *testing.T) {
	filtered := ByAliasExclude(indices, []string{"jaeger-span-read", "jaeger-span-other"})
	expected := []client.Index{
		{
			Index: "jaeger-span-0005",
			Aliases: map[string]bool{
				"custom-alias": true,
			},
		},
		{
			Index: "jaeger-span-0006",
		},
	}
	assert.Equal(t, expected, filtered)
}

func TestHasAliasEmpty(t *testing.T) {
	result := AliasExists(indices, "my-unexisting-alias")
	assert.False(t, result)

	result = AliasExists(indices, "custom-alias")
	assert.True(t, result)
}
