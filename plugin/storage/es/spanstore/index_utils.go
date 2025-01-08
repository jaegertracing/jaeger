// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"time"
)

// returns index name with date
func indexWithDate(indexPrefix, indexDateLayout string, date time.Time) string {
	spanDate := date.UTC().Format(indexDateLayout)
	return indexPrefix + spanDate
}
