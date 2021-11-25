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
	"fmt"
	"regexp"
	"time"

	"github.com/jaegertracing/jaeger/pkg/es/client"
	"github.com/jaegertracing/jaeger/pkg/es/filter"
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
	if i.Archive {
		// archive works only for rollover
		reg, _ = regexp.Compile(fmt.Sprintf("^%sjaeger-span-archive-\\d{6}", i.IndexPrefix))
	} else if i.Rollover {
		reg, _ = regexp.Compile(fmt.Sprintf("^%sjaeger-(span|service|dependencies)-\\d{6}", i.IndexPrefix))
	} else {
		reg, _ = regexp.Compile(fmt.Sprintf("^%sjaeger-(span|service|dependencies)-\\d{4}%s\\d{2}%s\\d{2}", i.IndexPrefix, i.IndexDateSeparator, i.IndexDateSeparator))
	}

	var filtered []client.Index
	for _, in := range indices {
		if reg.MatchString(in.Index) {
			// index in write alias cannot be removed
			if in.Aliases[i.IndexPrefix+"jaeger-span-write"] ||
				in.Aliases[i.IndexPrefix+"jaeger-service-write"] ||
				in.Aliases[i.IndexPrefix+"jaeger-span-archive-write"] ||
				in.Aliases[i.IndexPrefix+"jaeger-dependencies-write"] {
				continue
			}
			filtered = append(filtered, in)
		}
	}
	return filtered
}
