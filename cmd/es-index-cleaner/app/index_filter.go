// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
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
func (i *IndexFilter) Filter(indices []esclient.Index) []esclient.Index {
	indices = i.filterByPattern(indices)
	return filter.ByDate(indices, i.DeleteBeforeThisDate)
}

func (i *IndexFilter) filterByPattern(indices []esclient.Index) []esclient.Index {
	var reg *regexp.Regexp
	switch {
	case i.Archive:
		// archive works only for rollover
		reg, _ = regexp.Compile(fmt.Sprintf("^%sjaeger-span-archive-\\d{6}", i.IndexPrefix))
	case i.Rollover:
		reg, _ = regexp.Compile(fmt.Sprintf("^%sjaeger-(span|service|dependencies|sampling)-\\d{6}", i.IndexPrefix))
	default:
		sep := regexp.QuoteMeta(i.IndexDateSeparator)
		hourly := fmt.Sprintf(`\d{4}%s\d{2}%s\d{2}%s\d{2}`, sep, sep, sep)
		daily := fmt.Sprintf(`\d{4}%s\d{2}%s\d{2}`, sep, sep)
		monthly := fmt.Sprintf(`\d{4}%s\d{2}`, sep)
		yearly := `\d{4}`
		datePattern := strings.Join([]string{hourly, daily, monthly, yearly}, "|")
		// With an empty IndexDateSeparator, a monthly index's 6 digits collide
		// with a 6-digit rollover-alias ID; see TestIndexFilter_EmptySeparatorMonthAmbiguity.
		reg, _ = regexp.Compile(fmt.Sprintf(
			`^%sjaeger-(span|service|dependencies|sampling)-(%s)$`,
			regexp.QuoteMeta(i.IndexPrefix), datePattern,
		))
	}

	var filtered []esclient.Index
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
