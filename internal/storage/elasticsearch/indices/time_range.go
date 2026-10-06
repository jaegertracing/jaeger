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

// maxReadPeriods bounds how many dated index names a read enumerates. A wider range
// reads the whole index family through a wildcard and relies on the query's own
// time-range filter. 5000 periods is over 13 years of daily or 200 days of hourly
// indices, beyond realistic retention, so normal queries keep their exact index list.
const maxReadPeriods = 5000

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

// timeRangeIndices returns the array of indices that we need to query, based on query params
func timeRangeIndices(indexName, indexDateLayout string, startTime time.Time, endTime time.Time, reduceDuration time.Duration) []string {
	if endTime.Sub(startTime) > maxReadPeriods*-reduceDuration {
		return wideRangeIndices(indexName, indexDateLayout)
	}
	var result []string
	firstIndex := IndexWithDate(indexName, indexDateLayout, startTime)
	currentIndex := IndexWithDate(indexName, indexDateLayout, endTime)
	for currentIndex != firstIndex && endTime.After(startTime) {
		if len(result) == 0 || result[len(result)-1] != currentIndex {
			result = append(result, currentIndex)
		}
		endTime = endTime.Add(reduceDuration)
		currentIndex = IndexWithDate(indexName, indexDateLayout, endTime)
	}
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
