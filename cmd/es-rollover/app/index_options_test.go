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

package app

import (
	"testing"

	"github.com/crossdock/crossdock-go/assert"
)

func TestRolloverIndices(t *testing.T) {
	type expectedValues struct {
		mapping              string
		templateName         string
		readAliasName        string
		writeAliasName       string
		initialRolloverIndex string
	}

	tests := []struct {
		name     string
		archive  bool
		prefix   string
		expected []expectedValues
	}{
		{
			name: "Empty prefix",
			expected: []expectedValues{
				{
					templateName:         "jaeger-span",
					mapping:              "jaeger-span",
					readAliasName:        "jaeger-span-read",
					writeAliasName:       "jaeger-span-write",
					initialRolloverIndex: "jaeger-span-000001",
				},
				{
					templateName:         "jaeger-service",
					mapping:              "jaeger-service",
					readAliasName:        "jaeger-service-read",
					writeAliasName:       "jaeger-service-write",
					initialRolloverIndex: "jaeger-service-000001",
				},
				{
					templateName:         "jaeger-dependencies",
					mapping:              "jaeger-dependencies",
					readAliasName:        "jaeger-dependencies-read",
					writeAliasName:       "jaeger-dependencies-write",
					initialRolloverIndex: "jaeger-dependencies-000001",
				},
			},
		},
		{
			name:    "archive with prefix",
			archive: true,
			prefix:  "mytenant",
			expected: []expectedValues{
				{
					templateName:         "mytenant-jaeger-span",
					mapping:              "jaeger-span",
					readAliasName:        "mytenant-jaeger-span-archive-read",
					writeAliasName:       "mytenant-jaeger-span-archive-write",
					initialRolloverIndex: "mytenant-jaeger-span-archive-000001",
				},
			},
		},
		{
			name:    "archive empty prefix",
			archive: true,
			expected: []expectedValues{
				{
					mapping:              "jaeger-span",
					templateName:         "jaeger-span",
					readAliasName:        "jaeger-span-archive-read",
					writeAliasName:       "jaeger-span-archive-write",
					initialRolloverIndex: "jaeger-span-archive-000001",
				},
			},
		},
		{
			name:   "with prefix",
			prefix: "mytenant",
			expected: []expectedValues{
				{
					mapping:              "jaeger-span",
					templateName:         "mytenant-jaeger-span",
					readAliasName:        "mytenant-jaeger-span-read",
					writeAliasName:       "mytenant-jaeger-span-write",
					initialRolloverIndex: "mytenant-jaeger-span-000001",
				},
				{
					mapping:              "jaeger-service",
					templateName:         "mytenant-jaeger-service",
					readAliasName:        "mytenant-jaeger-service-read",
					writeAliasName:       "mytenant-jaeger-service-write",
					initialRolloverIndex: "mytenant-jaeger-service-000001",
				},
				{
					mapping:              "jaeger-dependencies",
					templateName:         "mytenant-jaeger-dependencies",
					readAliasName:        "mytenant-jaeger-dependencies-read",
					writeAliasName:       "mytenant-jaeger-dependencies-write",
					initialRolloverIndex: "mytenant-jaeger-dependencies-000001",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.prefix != "" {
				test.prefix += "-"
			}
			result := RolloverIndices(test.archive, test.prefix)
			for i, r := range result {
				assert.Equal(t, test.expected[i].templateName, r.TemplateName())
				assert.Equal(t, test.expected[i].mapping, r.Mapping)
				assert.Equal(t, test.expected[i].readAliasName, r.ReadAliasName())
				assert.Equal(t, test.expected[i].writeAliasName, r.WriteAliasName())
				assert.Equal(t, test.expected[i].initialRolloverIndex, r.InitialRolloverIndex())
			}
		})
	}
}
