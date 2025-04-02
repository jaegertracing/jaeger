// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		name             string
		archive          bool
		prefix           string
		skipDependencies bool
		adaptiveSampling bool
		expected         []expectedValues
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
			name:             "with prefix",
			prefix:           "mytenant",
			adaptiveSampling: true,
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
				{
					mapping:              "jaeger-sampling",
					templateName:         "mytenant-jaeger-sampling",
					readAliasName:        "mytenant-jaeger-sampling-read",
					writeAliasName:       "mytenant-jaeger-sampling-write",
					initialRolloverIndex: "mytenant-jaeger-sampling-000001",
				},
			},
		},
		{
			name:             "skip-dependency enable",
			prefix:           "mytenant",
			skipDependencies: true,
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
			},
		},
		{
			name:             "adaptive sampling enable",
			prefix:           "mytenant",
			skipDependencies: true,
			adaptiveSampling: true,
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
					mapping:              "jaeger-sampling",
					templateName:         "mytenant-jaeger-sampling",
					readAliasName:        "mytenant-jaeger-sampling-read",
					writeAliasName:       "mytenant-jaeger-sampling-write",
					initialRolloverIndex: "mytenant-jaeger-sampling-000001",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.prefix != "" {
				test.prefix += "-"
			}
			result := RolloverIndices(test.archive, test.skipDependencies, test.adaptiveSampling, test.prefix)
			assert.Len(t, result, len(test.expected))
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
