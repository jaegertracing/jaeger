// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"time"
)

// IndexWithDate returns index name with date
func IndexWithDate(indexPrefix, indexDateLayout string, date time.Time) string {
	return indexPrefix + date.UTC().Format(indexDateLayout)
}

// GetDataStreamLegacyWildcard returns the legacy wildcard pattern for a data stream.
// It replaces the first dot with a dash and appends a wildcard.
// Example: jaeger.span -> jaeger-span-*
func GetDataStreamLegacyWildcard(dataStreamName string) string {
	return strings.Replace(dataStreamName, ".", "-", 1) + "-*"
}
