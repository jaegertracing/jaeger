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
		// Matches yearly (YYYY), monthly (YYYY-SEP-MM), or daily (YYYY-SEP-MM-SEP-DD)
		// index suffixes, since periodic rotation now supports "year" and "month"
		// rollover_frequency in addition to "day" (and "hour", which is covered too
		// since the day segment is a prefix of the hour-suffixed name and this
		// pattern isn't end-anchored).
		//
		// The three alternatives are listed explicitly (rather than nesting them as
		// optional groups) and each is followed by \b. Go's regexp package (RE2)
		// has no lookahead, so a naive `\d{4}(SEP\d{2}(SEP\d{2})?)?` would let a bare
		// \d{4} satisfy the whole pattern, which would also match the first four
		// digits of unrelated sequential rollover-alias index names like
		// "jaeger-span-000001". The \b boundary after each alternative requires the
		// date portion to end at a non-digit (or end of string), ruling that out.
		reg, _ = regexp.Compile(fmt.Sprintf(
			"^%sjaeger-(span|service|dependencies|sampling)-(\\d{4}%s\\d{2}%s\\d{2}|\\d{4}%s\\d{2}|\\d{4})\\b",
			i.IndexPrefix, i.IndexDateSeparator, i.IndexDateSeparator, i.IndexDateSeparator,
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
