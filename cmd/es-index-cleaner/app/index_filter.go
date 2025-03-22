// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"regexp"
	"time"

	"github.com/jaegertracing/jaeger/internal/storage/es/client"
	"github.com/jaegertracing/jaeger/internal/storage/es/filter"
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
	indices = i.filter(indices)
	return filter.ByDate(indices, i.DeleteBeforeThisDate)
}

func (i *IndexFilter) filter(indices []client.Index) []client.Index {
	var reg *regexp.Regexp
	switch {
	case i.Archive:
		// archive works only for rollover
		reg, _ = regexp.Compile(fmt.Sprintf("^%sjaeger-span-archive-\\d{6}", i.IndexPrefix))
	case i.Rollover:
		reg, _ = regexp.Compile(fmt.Sprintf("^%sjaeger-(span|service|dependencies|sampling)-\\d{6}", i.IndexPrefix))
	default:
		reg, _ = regexp.Compile(fmt.Sprintf("^%sjaeger-(span|service|dependencies|sampling)-\\d{4}%s\\d{2}%s\\d{2}", i.IndexPrefix, i.IndexDateSeparator, i.IndexDateSeparator))
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
