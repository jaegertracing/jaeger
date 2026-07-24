// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

// TestIndexFilter_MonthlyAndYearlyIndices checks that the default filter
// pattern matches monthly and yearly index suffixes, and does not
// false-match rollover-alias-style names like "jaeger-span-000001".
func TestIndexFilter_MonthlyAndYearlyIndices(t *testing.T) {
	beforeDate := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)
	indices := []client.Index{
		{
			// Yearly index, created before the cutoff: should be deleted.
			Index:        "jaeger-span-2023",
			CreationTime: time.Date(2023, time.December, 31, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			// Yearly index, created after the cutoff: should be kept.
			Index:        "jaeger-span-2024",
			CreationTime: time.Date(2024, time.June, 15, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			// Monthly index, created before the cutoff: should be deleted.
			Index:        "jaeger-service-2024-04",
			CreationTime: time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			// Monthly index, created after the cutoff: should be kept.
			Index:        "jaeger-service-2024-06",
			CreationTime: time.Date(2024, time.June, 10, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			// Sequential rollover-alias style index (e.g. from a Rollover=true
			// config) must NOT be matched by the default (periodic) filter, even
			// though it starts with 4 digits.
			Index:        "jaeger-span-000001",
			CreationTime: time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC),
			Aliases: map[string]bool{
				"jaeger-span-read": true,
			},
		},
	}

	filter := &IndexFilter{
		IndexPrefix:          "",
		IndexDateSeparator:   "-",
		Archive:              false,
		Rollover:             false,
		DeleteBeforeThisDate: beforeDate,
	}

	filtered := filter.Filter(indices)
	assert.Equal(t, []client.Index{
		{
			Index:        "jaeger-span-2023",
			CreationTime: time.Date(2023, time.December, 31, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        "jaeger-service-2024-04",
			CreationTime: time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
	}, filtered)
}

func TestIndexFilter_HourlyIndicesWithNonDashSeparators(t *testing.T) {
	beforeDate := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name      string
		separator string
		indexName string
	}{
		{name: "underscore separator, hourly index", separator: "_", indexName: "jaeger-span-2024_03_15_12"},
		{name: "empty separator, hourly index", separator: "", indexName: "jaeger-span-2024031512"},
		{name: "underscore separator, daily index", separator: "_", indexName: "jaeger-span-2024_03_15"},
		{name: "empty separator, daily index", separator: "", indexName: "jaeger-span-20240315"},
		{name: "empty separator, yearly index", separator: "", indexName: "jaeger-span-2024"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter := &IndexFilter{
				IndexPrefix:          "",
				IndexDateSeparator:   tc.separator,
				Archive:              false,
				Rollover:             false,
				DeleteBeforeThisDate: beforeDate,
			}
			indices := []client.Index{
				{
					Index:        tc.indexName,
					CreationTime: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
			}
			filtered := filter.Filter(indices)
			require.Len(t, filtered, 1, "expected %q to match the default periodic pattern and be selected for deletion", tc.indexName)
			assert.Equal(t, tc.indexName, filtered[0].Index)
		})
	}
}

// TestIndexFilter_EmptySeparatorMonthAmbiguity documents a known limitation:
// with an empty IndexDateSeparator, a monthly index's 6 digits (e.g.
// "202403") are indistinguishable from a 6-digit rollover-alias ID (e.g.
// "000001") — there's no delimiter left to tell them apart.
func TestIndexFilter_EmptySeparatorMonthAmbiguity(t *testing.T) {
	beforeDate := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)
	filter := &IndexFilter{
		IndexPrefix:          "",
		IndexDateSeparator:   "",
		Archive:              false,
		Rollover:             false,
		DeleteBeforeThisDate: beforeDate,
	}
	indices := []client.Index{
		{
			Index:        "jaeger-span-202404",
			CreationTime: time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        "jaeger-span-000001",
			CreationTime: time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
	}
	filtered := filter.Filter(indices)
	assert.Len(t, filtered, 2, "documented limitation: both the monthly index and the 6-digit rollover-alias-style index match when IndexDateSeparator is empty")
}

func TestIndexFilter_RegexSpecialSeparatorsAreLiteral(t *testing.T) {
	beforeDate := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name      string
		separator string
	}{
		{name: "dot separator", separator: "."},
		{name: "plus separator", separator: "+"},
		{name: "bracket separator", separator: "["},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter := &IndexFilter{
				IndexPrefix:          "",
				IndexDateSeparator:   tc.separator,
				Archive:              false,
				Rollover:             false,
				DeleteBeforeThisDate: beforeDate,
			}
			indexName := fmt.Sprintf("jaeger-span-2024%s03%s15", tc.separator, tc.separator)
			indices := []client.Index{
				{
					Index:        indexName,
					CreationTime: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
			}
			filtered := filter.Filter(indices)
			require.Len(t, filtered, 1, "expected %q to match its own literal separator", indexName)
		})
	}
}

func TestIndexFilter_WrongSeparatorOrUnrelatedNamesAreNotDeleted(t *testing.T) {
	beforeDate := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)
	filter := &IndexFilter{
		IndexPrefix:          "",
		IndexDateSeparator:   "-",
		Archive:              false,
		Rollover:             false,
		DeleteBeforeThisDate: beforeDate,
	}

	testCases := []string{
		"jaeger-span-2024.03.15",
		"jaeger-span-2024_03_15",
		"jaeger-span-2024-backup",
		"jaeger-span-archive",
	}
	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			indices := []client.Index{
				{
					Index:        name,
					CreationTime: time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC),
					Aliases:      map[string]bool{},
				},
			}
			filtered := filter.Filter(indices)
			assert.Empty(t, filtered, "%q should not be selected for deletion", name)
		})
	}
}

func TestIndexFilter_MultiYearReadCoveringLeapYear(t *testing.T) {
	beforeDate := time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
	filter := &IndexFilter{
		IndexPrefix:          "",
		IndexDateSeparator:   "-",
		Archive:              false,
		Rollover:             false,
		DeleteBeforeThisDate: beforeDate,
	}
	indices := []client.Index{
		{
			Index:        "jaeger-span-2019",
			CreationTime: time.Date(2019, time.June, 1, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        "jaeger-span-2020",
			CreationTime: time.Date(2020, time.February, 29, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        "jaeger-span-2021",
			CreationTime: time.Date(2021, time.March, 1, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
	}
	filtered := filter.Filter(indices)
	assert.Equal(t, []client.Index{
		{
			Index:        "jaeger-span-2019",
			CreationTime: time.Date(2019, time.June, 1, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
		{
			Index:        "jaeger-span-2020",
			CreationTime: time.Date(2020, time.February, 29, 0, 0, 0, 0, time.UTC),
			Aliases:      map[string]bool{},
		},
	}, filtered)
}

func TestIndexFilter(t *testing.T) {
	runIndexFilterTest(t, "")
}

func TestIndexFilterWithPrefix(t *testing.T) {
	runIndexFilterTest(t, "tenant1-")
}

func runIndexFilterTest(t *testing.T, prefix string) {
	time20200807 := time.Date(2020, time.August, 6, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	indices := []esclient.Index{
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
		expected []esclient.Index
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
			expected: []esclient.Index{
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
			expected: []esclient.Index{
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
			expected: []esclient.Index{
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
			expected: []esclient.Index{
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
			expected: []esclient.Index{
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
			expected: []esclient.Index{
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
			expected: []esclient.Index{
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
