// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"regexp"
	"time"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/client"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/filter"
)

// IndexFilter holds configuration for index filtering.
type IndexFilter struct {
	// Index prefix.
	IndexPrefix string
	// Separator between date fragments.
	IndexDateSeparator string
	// Whether to filter archive indices.
	Archive bool
	// Whether to filter rollover indices.
	Rollover bool
	// Indices created before this date will be deleted.
	DeleteBeforeThisDate time.Time
}

// Filter filters indices.
func (i *IndexFilter) Filter(indices []client.Index) []client.Index {
	indices = i.filterByPattern(indices)
	return filter.ByDate(indices, i.DeleteBeforeThisDate)
}

func (i *IndexFilter) filterByPattern(indices []client.Index) []client.Index {
	var reg *regexp.Regexp
	switch {
	case i.Archive:
		// archive works only for rollover
		reg, _ = regexp.Compile(fmt.Sprintf("^%sjaeger-span-archive-\\d{6}", i.IndexPrefix))
	case i.Rollover:
		reg, _ = regexp.Compile(fmt.Sprintf("^%sjaeger-(span|service|dependencies|sampling)-\\d{6}", i.IndexPrefix))
	default:
		// Matches yearly (YYYY), monthly (YYYY-SEP-MM), daily (YYYY-SEP-MM-SEP-DD),
		// or hourly (YYYY-SEP-MM-SEP-DD-SEP-HH) index suffixes, since periodic
		// rotation now supports "year" and "month" rollover_frequency in addition
		// to "day" and "hour".
		//
		// The four alternatives are listed explicitly (rather than nesting them as
		// optional groups) so each can be followed by a boundary assertion. Go's
		// regexp package (RE2) has no lookahead, so a naive
		// `\d{4}(SEP\d{2}(SEP\d{2})?)?` would let a bare \d{4} satisfy the whole
		// pattern, which would also match the first four digits of unrelated
		// sequential rollover-alias index names like "jaeger-span-000001".
		//
		// The boundary is `([^0-9]|$)` — "not a digit, or end of string" — rather
		// than `\b`. `\b` depends on Go's \w definition, which treats digits AND
		// underscore as word characters, so it fails to find a boundary between a
		// date segment and an underscore-separated (or unseparated) suffix; that
		// silently excluded valid indices like "jaeger-span-2024_03_15_12" or
		// "jaeger-span-2024031512" from matching at all.
		//
		// Known limitation: if IndexDateSeparator is "" (no separator character
		// between date segments at all), a monthly index's 6 raw digits
		// (e.g. "202403") are indistinguishable by pattern alone from a 6-digit
		// rollover-alias sequential ID (e.g. "000001", matched by the Rollover
		// case above) — there is no delimiting character left to tell them apart.
		// This is a pre-existing ambiguity in the empty-separator case, not
		// something introduced here; it only matters if periodic-month-frequency
		// and rollover-alias indices coexist under the same prefix, which a
		// single IndexFilter run (Rollover is one bool for the whole run) doesn't
		// normally produce. Year, day, and hour granularities are unambiguous
		// even with an empty separator, since their widths (4, 8, 10 digits)
		// don't collide with the 6-digit rollover-alias pattern.
		reg, _ = regexp.Compile(fmt.Sprintf(
			"^%sjaeger-(span|service|dependencies|sampling)-(\\d{4}%s\\d{2}%s\\d{2}%s\\d{2}|\\d{4}%s\\d{2}%s\\d{2}|\\d{4}%s\\d{2}|\\d{4})([^0-9]|$)",
			i.IndexPrefix,
			i.IndexDateSeparator, i.IndexDateSeparator, i.IndexDateSeparator,
			i.IndexDateSeparator, i.IndexDateSeparator,
			i.IndexDateSeparator,
		))
	}

	var filtered []client.Index
	for _, in := range indices {
		if reg.MatchString(in.Index) {
			// index in write alias cannot be removed
			if in.Aliases[i.IndexPrefix+"jaeger-span-write"] ||
				in.Aliases[i.IndexPrefix+"jaeger-service-write"] ||
				in.Aliases[i.IndexPrefix+"jaeger-span-archive-write"] ||
				in.Aliases[i.IndexPrefix+"jaeger-dependencies-write"] ||
				in.Aliases[i.IndexPrefix+"jaeger-sampling-write"] {
				continue
			}
			filtered = append(filtered, in)
		}
	}
	return filtered
}
