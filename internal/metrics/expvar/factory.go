// Copyright (c) 2022 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
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

package expvar

import (
	"sort"

	kexpvar "github.com/go-kit/kit/metrics/expvar"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// NewFactory creates a new metrics factory using go-kit expvar package.
// buckets is the number of buckets to be used in histograms.
// Custom buckets passed via options are not supported.
func NewFactory(buckets int) metrics.Factory {
	return &factory{
		buckets:  buckets,
		scope:    "",
		scopeSep: ".",
		tagsSep:  ".",
		tagKVSep: "_",
		cache:    newCache(),
	}
}

type factory struct {
	buckets int

	scope    string
	tags     map[string]string
	scopeSep string
	tagsSep  string
	tagKVSep string
	cache    *cache
}

var _ metrics.Factory = (*factory)(nil)

func (f *factory) subScope(name string) string {
	if f.scope == "" {
		return name
	}
	if name == "" {
		return f.scope
	}
	return f.scope + f.scopeSep + name
}

func (f *factory) mergeTags(tags map[string]string) map[string]string {
	ret := make(map[string]string, len(f.tags)+len(tags))
	for k, v := range f.tags {
		ret[k] = v
	}
	for k, v := range tags {
		ret[k] = v
	}
	return ret
}

func (f *factory) getKey(name string, tags map[string]string) string {
	fullName := f.subScope(name)
	fullTags := f.mergeTags(tags)
	return makeKey(fullName, fullTags, f.tagsSep, f.tagKVSep)
}

// getKey converts name+tags into a single string of the form
// "name|tag1=value1|...|tagN=valueN", where tag names are
// sorted alphabetically.
func makeKey(name string, tags map[string]string, tagsSep string, tagKVSep string) string {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	key := name
	for _, k := range keys {
		key = key + tagsSep + k + tagKVSep + tags[k]
	}
	return key
}

func (f *factory) Counter(options metrics.Options) metrics.Counter {
	key := f.getKey(options.Name, options.Tags)
	return f.cache.getOrSetCounter(key, func() metrics.Counter {
		return NewCounter(kexpvar.NewCounter(key))
	})
}

func (f *factory) Gauge(options metrics.Options) metrics.Gauge {
	key := f.getKey(options.Name, options.Tags)
	return f.cache.getOrSetGauge(key, func() metrics.Gauge {
		return NewGauge(kexpvar.NewGauge(key))
	})
}

func (f *factory) Timer(options metrics.TimerOptions) metrics.Timer {
	key := f.getKey(options.Name, options.Tags)
	return f.cache.getOrSetTimer(key, func() metrics.Timer {
		return NewTimer(kexpvar.NewHistogram(key, f.buckets))
	})
}

func (f *factory) Histogram(options metrics.HistogramOptions) metrics.Histogram {
	key := f.getKey(options.Name, options.Tags)
	return f.cache.getOrSetHistogram(key, func() metrics.Histogram {
		return NewHistogram(kexpvar.NewHistogram(key, f.buckets))
	})
}

func (f *factory) Namespace(options metrics.NSOptions) metrics.Factory {
	return &factory{
		buckets:  f.buckets,
		scope:    f.subScope(options.Name),
		tags:     f.mergeTags(options.Tags),
		scopeSep: f.scopeSep,
		tagsSep:  f.tagsSep,
		tagKVSep: f.tagKVSep,
		cache:    f.cache,
	}
}
