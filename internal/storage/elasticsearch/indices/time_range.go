// Copyright (c) 2018 The Jaeger Authors.
// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// maxReadTargetsLen bounds the length, in bytes, of the comma-joined index list a read
// enumerates. The list goes into the request line
// (POST /<indices>/_search?<params> HTTP/1.1), which Elasticsearch and OpenSearch
// reject when it is longer than http.max_initial_line_length, 4 KB by default. The
// method, query string and protocol take about 75 bytes of that, and the rest is
// headroom. With the default span index names this is 115 hourly or 130 daily indices.
// A range whose list would be longer is not joined. ReadTargets then reads the whole
// index family through wildcards and relies on the query's own time-range filter.
// ExactTargets still returns concrete names; see maxExactTargets.
const maxReadTargetsLen = 3000

// maxExactTargets caps the concrete names ExactTargets returns once a range no longer
// fits in maxReadTargetsLen. Those callers probe one name at a time, so the request-line
// budget does not apply, but the list still has to stay bounded: an unbounded lookback
// would allocate one name per period. 4096 hourly periods is about 170 days, and 4096
// daily periods is about eleven years. Names past the cap are the older ones.
const maxExactTargets = 4096

// TimeRangeIndexFn is a function that returns the list of index names for a given time range.
type TimeRangeIndexFn func(indexName string, indexDateLayout string, startTime time.Time, endTime time.Time, reduceDuration time.Duration) []string

// LoggingTimeRangeIndexFn wraps a TimeRangeIndexFn with debug logging.
func LoggingTimeRangeIndexFn(logger *zap.Logger, fn TimeRangeIndexFn) TimeRangeIndexFn {
	if !logger.Core().Enabled(zap.DebugLevel) {
		return fn
	}
	return func(indexName string, indexDateLayout string, startTime time.Time, endTime time.Time, reduceDuration time.Duration) []string {
		indices := fn(indexName, indexDateLayout, startTime, endTime, reduceDuration)
		logger.Debug("Reading from ES indices", zap.Strings("index", indices))
		return indices
	}
}

// TimeRangeIndicesFn returns a TimeRangeIndexFn configured for the given read settings.
func TimeRangeIndicesFn(useReadWriteAliases bool, readAliasSuffix string, remoteReadClusters []string) TimeRangeIndexFn {
	suffix := ""
	if useReadWriteAliases {
		if readAliasSuffix != "" {
			suffix = readAliasSuffix
		} else {
			suffix = "read"
		}
	}
	return addRemoteReadClusters(
		getTimeRangeIndexFn(useReadWriteAliases, suffix),
		remoteReadClusters,
	)
}

func getTimeRangeIndexFn(useReadWriteAliases bool, readAlias string) TimeRangeIndexFn {
	if useReadWriteAliases {
		return func(indexPrefix, _ /* indexDateLayout */ string, _ /* startTime */ time.Time, _ /* endTime */ time.Time, _ /* reduceDuration */ time.Duration) []string {
			return []string{indexPrefix + config.IndexSeparator + readAlias}
		}
	}
	return timeRangeIndices
}

// Add a remote cluster prefix for each cluster and for each index and add it to the list of original indices.
// Elasticsearch cross cluster api example GET /twitter,cluster_one:twitter,cluster_two:twitter/_search.
func addRemoteReadClusters(fn TimeRangeIndexFn, remoteReadClusters []string) TimeRangeIndexFn {
	return func(indexPrefix string, indexDateLayout string, startTime time.Time, endTime time.Time, reduceDuration time.Duration) []string {
		jaegerIndices := fn(indexPrefix, indexDateLayout, startTime, endTime, reduceDuration)
		if len(remoteReadClusters) == 0 {
			return jaegerIndices
		}

		for _, jaegerIndex := range jaegerIndices {
			for _, remoteCluster := range remoteReadClusters {
				remoteIndex := remoteCluster + ":" + jaegerIndex
				jaegerIndices = append(jaegerIndices, remoteIndex)
			}
		}

		return jaegerIndices
	}
}

// timeRangeIndices returns the array of indices that we need to query, based on query params.
// A range that would exceed the request-line budget is returned as wildcards.
func timeRangeIndices(indexName, indexDateLayout string, startTime time.Time, endTime time.Time, reduceDuration time.Duration) []string {
	firstIndex := IndexWithDate(indexName, indexDateLayout, startTime)
	if readTargetsExceedBudget(firstIndex, startTime, endTime, reduceDuration) {
		return wideRangeIndices(indexName, indexDateLayout)
	}
	return enumerateTimeRange(indexName, indexDateLayout, startTime, endTime, reduceDuration, 0)
}

// exactTimeRangeIndices returns concrete index names for the same range, newest first.
// It never returns a wildcard. Past the request-line budget it keeps the newest
// maxExactTargets names instead of the patterns ReadTargets uses.
func exactTimeRangeIndices(indexName, indexDateLayout string, startTime time.Time, endTime time.Time, reduceDuration time.Duration) []string {
	firstIndex := IndexWithDate(indexName, indexDateLayout, startTime)
	if readTargetsExceedBudget(firstIndex, startTime, endTime, reduceDuration) {
		return enumerateTimeRange(indexName, indexDateLayout, startTime, endTime, reduceDuration, maxExactTargets)
	}
	return enumerateTimeRange(indexName, indexDateLayout, startTime, endTime, reduceDuration, 0)
}

// readTargetsExceedBudget reports whether joining one name per period would exceed
// maxReadTargetsLen. A range spanning n whole or partial periods touches at most
// n+1 names, each adding its length plus a comma. periods is that count minus the
// first name, so the list is longer than maxNames once periods reaches maxNames.
func readTargetsExceedBudget(firstIndex string, startTime, endTime time.Time, reduceDuration time.Duration) bool {
	maxNames := maxReadTargetsLen / (len(firstIndex) + 1)
	span, period := endTime.Sub(startTime), -reduceDuration
	periods := int64(span / period)
	if span%period != 0 {
		periods++
	}
	return periods >= int64(maxNames)
}

// enumerateTimeRange lists concrete index names from endTime backward. limit caps how
// many names are returned; zero means no cap. reduceDuration is negative (one rollover
// period toward the past). The newest name is first. The oldest name in range is
// included when the cap has room for it.
func enumerateTimeRange(indexName, indexDateLayout string, startTime, endTime time.Time, reduceDuration time.Duration, limit int) []string {
	firstIndex := IndexWithDate(indexName, indexDateLayout, startTime)
	var result []string
	if limit > 0 {
		result = make([]string, 0, limit)
	}
	currentIndex := IndexWithDate(indexName, indexDateLayout, endTime)
	for currentIndex != firstIndex && endTime.After(startTime) {
		if len(result) == 0 || result[len(result)-1] != currentIndex {
			result = append(result, currentIndex)
			if limit > 0 && len(result) == limit {
				return result
			}
		}
		next := endTime.Add(reduceDuration)
		// A zero or positive step would otherwise spin on the same timestamp.
		if !next.Before(endTime) {
			break
		}
		endTime = next
		currentIndex = IndexWithDate(indexName, indexDateLayout, endTime)
	}
	// The cap returns above, so this is the oldest name still inside the range.
	result = append(result, firstIndex)
	return result
}

// wideRangeIndices matches every dated index of the family. When the layout starts with
// the year, the patterns also require a date-shaped suffix, so archive indices, aliases
// and rollover indices that share the prefix (e.g. jaeger-span-archive-000001) are not read.
func wideRangeIndices(indexName, indexDateLayout string) []string {
	prefix := indexName + config.IndexSeparator
	if strings.HasPrefix(indexDateLayout, "2006") {
		return []string{prefix + "1*", prefix + "2*"}
	}
	return []string{prefix + "*"}
}

// IndexWithDate returns index name with date
func IndexWithDate(indexPrefix, indexDateLayout string, date time.Time) string {
	spanDate := date.UTC().Format(indexDateLayout)
	return indexPrefix + config.IndexSeparator + spanDate
}
