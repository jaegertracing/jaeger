// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"time"

	cfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// IndexWithDate returns index name with date
//
// Deprecated: use cfg.IndexWithDate instead
func indexWithDate(indexPrefix, indexDateLayout string, date time.Time) string { //nolint:unused // proxy for user visibility while using centralized logic
	return cfg.IndexWithDate(indexPrefix, indexDateLayout, date)
}
