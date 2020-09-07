// Copyright (c) 2020 The Jaeger Authors.
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

package storagemetrics

import (
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	tagServiceName = tag.MustNewKey("service")
	tagExporter    = tag.MustNewKey("exporter")

	statSpanStoredCount    = stats.Int64("storage_exporter_stored_spans", "Number of stored spans", stats.UnitDimensionless)
	statSpanNotStoredCount = stats.Int64("storage_exporter_not_stored_spans", "Number of spans that failed to be stored", stats.UnitDimensionless)
)

// MetricViews returns the metrics views related to storage.
func MetricViews() []*view.View {
	tags := []tag.Key{tagServiceName, tagExporter}

	countSpanStoredView := &view.View{
		Name:        statSpanStoredCount.Name(),
		Measure:     statSpanStoredCount,
		Description: statSpanStoredCount.Description(),
		TagKeys:     tags,
		Aggregation: view.Sum(),
	}
	countSpanNotStoredView := &view.View{
		Name:        statSpanNotStoredCount.Name(),
		Measure:     statSpanNotStoredCount,
		Description: statSpanNotStoredCount.Description(),
		TagKeys:     tags,
		Aggregation: view.Sum(),
	}

	return []*view.View{
		countSpanStoredView,
		countSpanNotStoredView,
	}
}

// TagServiceName returns spans's service name tag.
func TagServiceName() tag.Key {
	return tagServiceName
}

// TagExporterName returns exporter name tag.
func TagExporterName() tag.Key {
	return tagExporter
}

// StatSpansStoredCount returns counter for spans that were successfully stored.
func StatSpansStoredCount() *stats.Int64Measure {
	return statSpanStoredCount
}

// StatSpansNotStoredCount returns counter for spans that failed to be stored.
func StatSpansNotStoredCount() *stats.Int64Measure {
	return statSpanNotStoredCount
}
