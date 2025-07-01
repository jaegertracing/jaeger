// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/client"
)

func TestIndexFilter(t *testing.T) {
	testIndexFilter(t, "")
}

func TestIndexFilter_prefix(t *testing.T) {
	testIndexFilter(t, "tenant1-")
}

func testIndexFilter(t *testing.T, prefix string) {
	time20200807 := time.Date(2020, time.August, 6, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	indices := []client.Index{
		{
			Index:        prefix + "jaeger-span-2020-08-06",
			CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        prefix + "jaeger-span-2020-08-05",
			CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        prefix + "jaeger-service-2020-08-06",
			CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        prefix + "jaeger-service-2020-08-05",
			CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        prefix + "jaeger-dependencies-2020-08-06",
			CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        prefix + "jaeger-dependencies-2020-08-05",
			CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        prefix + "jaeger-sampling-2020-08-06",
			CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        prefix + "jaeger-sampling-2020-08-05",
			CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        prefix + "jaeger-span-archive",
			CreationTime: time.Date(2020, time.August, 1, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        prefix + "jaeger-span-000001",
			CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
			Aliases: map[string]bool{
				prefix + "jaeger-span-read": true,
			},
		},
		{
			Index:        prefix + "jaeger-span-000002",
			CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
			Aliases: map[string]bool{
				prefix + "jaeger-span-read":  true,
				prefix + "jaeger-span-write": true,
			},
		},
		{
			Index:        prefix + "jaeger-service-000001",
			CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
			Aliases: map[string]bool{
				prefix + "jaeger-service-read": true,
			},
		},
		{
			Index:        prefix + "jaeger-service-000002",
			CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
			Aliases: map[string]bool{
				prefix + "jaeger-service-read":  true,
				prefix + "jaeger-service-write": true,
			},
		},
		{
			Index:        prefix + "jaeger-span-archive-000001",
			CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
			Aliases: map[string]bool{
				prefix + "jaeger-span-archive-read": true,
			},
		},
		{
			Index:        prefix + "jaeger-span-archive-000002",
			CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
			Aliases: map[string]bool{
				prefix + "jaeger-span-archive-read":  true,
				prefix + "jaeger-span-archive-write": true,
			},
		},
		{
			Index:        "other-jaeger-span-2020-08-05",
			CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        "other-jaeger-service-2020-08-06",
			CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        "other-bar-jaeger-span-000002",
			CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
			Aliases: map[string]bool{
				"other-jaeger-span-read":  true,
				"other-jaeger-span-write": true,
			},
		},
		{
			Index:        "otherfoo-jaeger-span-archive",
			CreationTime: time.Date(2020, time.August, 1, 15, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        "foo-jaeger-span-archive-000001",
			CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
			Aliases: map[string]bool{
				"foo-jaeger-span-archive-read": true,
			},
		},
	}

	tests := []struct {
		name     string
		filter   *IndexFilter
		expected []client.Index
	}{
		{
			name: "normal indices, remove older than 2 days",
			filter: &IndexFilter{
				IndexPrefix:          prefix,
				IndexDateSeparator:   "-",
				Archive:              false,
				Rollover:             false,
				DeleteBeforeThisDate: time20200807.Add(-time.Hour * 24 * time.Duration(2)),
			},
		},
		{
			name: "normal indices, remove older 1 days",
			filter: &IndexFilter{
				IndexPrefix:          prefix,
				IndexDateSeparator:   "-",
				Archive:              false,
				Rollover:             false,
				DeleteBeforeThisDate: time20200807.Add(-time.Hour * 24 * time.Duration(1)),
			},
			expected: []client.Index{
				{
					Index:        prefix + "jaeger-span-2020-08-05",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
				{
					Index:        prefix + "jaeger-service-2020-08-05",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
				{
					Index:        prefix + "jaeger-dependencies-2020-08-05",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
				{
					Index:        prefix + "jaeger-sampling-2020-08-05",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
			},
		},
		{
			name: "normal indices, remove older 0 days - it should remove all indices",
			filter: &IndexFilter{
				IndexPrefix:          prefix,
				IndexDateSeparator:   "-",
				Archive:              false,
				Rollover:             false,
				DeleteBeforeThisDate: time20200807.Add(-time.Hour * 24 * time.Duration(0)),
			},
			expected: []client.Index{
				{
					Index:        prefix + "jaeger-span-2020-08-06",
					CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
				{
					Index:        prefix + "jaeger-span-2020-08-05",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
				{
					Index:        prefix + "jaeger-service-2020-08-06",
					CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
				{
					Index:        prefix + "jaeger-service-2020-08-05",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
				{
					Index:        prefix + "jaeger-dependencies-2020-08-06",
					CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
				{
					Index:        prefix + "jaeger-dependencies-2020-08-05",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
				{
					Index:        prefix + "jaeger-sampling-2020-08-06",
					CreationTime: time.Date(2020, time.August, 6, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
				{
					Index:        prefix + "jaeger-sampling-2020-08-05",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
			},
		},
		{
			name: "archive indices, remove older 1 days - archive works only for rollover",
			filter: &IndexFilter{
				IndexPrefix:          prefix,
				IndexDateSeparator:   "-",
				Archive:              true,
				Rollover:             false,
				DeleteBeforeThisDate: time20200807.Add(-time.Hour * 24 * time.Duration(1)),
			},
			expected: []client.Index{
				{
					Index:        prefix + "jaeger-span-archive-000001",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases: map[string]bool{
						prefix + "jaeger-span-archive-read": true,
					},
				},
			},
		},
		{
			name: "rollover indices, remove older 1 days",
			filter: &IndexFilter{
				IndexPrefix:          prefix,
				IndexDateSeparator:   "-",
				Archive:              false,
				Rollover:             true,
				DeleteBeforeThisDate: time20200807.Add(-time.Hour * 24 * time.Duration(1)),
			},
			expected: []client.Index{
				{
					Index:        prefix + "jaeger-span-000001",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases: map[string]bool{
						prefix + "jaeger-span-read": true,
					},
				},
				{
					Index:        prefix + "jaeger-service-000001",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases: map[string]bool{
						prefix + "jaeger-service-read": true,
					},
				},
			},
		},
		{
			name: "rollover indices, remove older 0 days, index in write alias cannot be removed",
			filter: &IndexFilter{
				IndexPrefix:          prefix,
				IndexDateSeparator:   "-",
				Archive:              false,
				Rollover:             true,
				DeleteBeforeThisDate: time20200807.Add(-time.Hour * 24 * time.Duration(0)),
			},
			expected: []client.Index{
				{
					Index:        prefix + "jaeger-span-000001",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases: map[string]bool{
						prefix + "jaeger-span-read": true,
					},
				},
				{
					Index:        prefix + "jaeger-service-000001",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases: map[string]bool{
						prefix + "jaeger-service-read": true,
					},
				},
			},
		},
		{
			name: "rollover archive indices, remove older 1 days",
			filter: &IndexFilter{
				IndexPrefix:          prefix,
				IndexDateSeparator:   "-",
				Archive:              true,
				Rollover:             true,
				DeleteBeforeThisDate: time20200807.Add(-time.Hour * 24 * time.Duration(1)),
			},
			expected: []client.Index{
				{
					Index:        prefix + "jaeger-span-archive-000001",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases: map[string]bool{
						prefix + "jaeger-span-archive-read": true,
					},
				},
			},
		},
		{
			name: "rollover archive indices, remove older 0 days, index in write alias cannot be removed",
			filter: &IndexFilter{
				IndexPrefix:          prefix,
				IndexDateSeparator:   "-",
				Archive:              true,
				Rollover:             true,
				DeleteBeforeThisDate: time20200807.Add(-time.Hour * 24 * time.Duration(0)),
			},
			expected: []client.Index{
				{
					Index:        prefix + "jaeger-span-archive-000001",
					CreationTime: time.Date(2020, time.August, 5, 15, 0, 0, 0, time.UTC),
					Aliases: map[string]bool{
						prefix + "jaeger-span-archive-read": true,
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indices := test.filter.Filter(indices)
			assert.Equal(t, test.expected, indices)
		})
	}
}
