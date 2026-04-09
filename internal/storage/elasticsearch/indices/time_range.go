// Copyright (c) 2018 The Jaeger Authors.
// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"time"

	"go.uber.org/zap"
)

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
			return []string{indexPrefix + readAlias}
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

// IndexWithDate returns index name with date
func IndexWithDate(indexPrefix, indexDateLayout string, date time.Time) string {
	spanDate := date.UTC().Format(indexDateLayout)
	return indexPrefix + spanDate
}
