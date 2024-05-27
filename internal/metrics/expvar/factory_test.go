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
	"expvar"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

var (
	id              = time.Now().UnixNano()
	prefix          = fmt.Sprintf("test_%d", id)
	counterPrefix   = prefix + "_counter_"
	gaugePrefix     = prefix + "_gauge_"
	timerPrefix     = prefix + "_timer_"
	histogramPrefix = prefix + "_histogram_"

	tagsA = map[string]string{"a": "b"}
	tagsX = map[string]string{"x": "y"}
)

func TestFactory(t *testing.T) {
	testCases := []struct {
		name            string
		tags            map[string]string
		durationBuckets []time.Duration
		namespace       string
		nsTags          map[string]string
		fullName        string
		expectedCounter string
	}{
		{name: "x", fullName: "%sx"},
		{name: "x", fullName: "%sx", expectedCounter: "84"}, // same as above to validate that the same vars are ok to create
		{tags: tagsX, fullName: "%s.x_y"},
		{name: "x", tags: tagsA, fullName: "%sx.a_b"},
		{namespace: "y", fullName: "y.%s"},
		{nsTags: tagsA, fullName: "%s.a_b"},
		{namespace: "y", nsTags: tagsX, fullName: "y.%s.x_y"},
		{name: "x", namespace: "y", nsTags: tagsX, fullName: "y.%sx.x_y"},
		{name: "x", tags: tagsX, namespace: "y", nsTags: tagsX, fullName: "y.%sx.x_y", expectedCounter: "84"},
		{name: "x", tags: tagsA, namespace: "y", nsTags: tagsX, fullName: "y.%sx.a_b.x_y"},
		{name: "x", tags: tagsX, namespace: "y", nsTags: tagsA, fullName: "y.%sx.a_b.x_y", expectedCounter: "84"},
	}
	f := NewFactory()
	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			if testCase.expectedCounter == "" {
				testCase.expectedCounter = "42"
			}
			ff := f
			if testCase.namespace != "" || testCase.nsTags != nil {
				ff = f.Namespace(metrics.NSOptions{
					Name: testCase.namespace,
					Tags: testCase.nsTags,
				})
			}
			counter := ff.Counter(metrics.Options{
				Name: counterPrefix + testCase.name,
				Tags: testCase.tags,
			})
			gauge := ff.Gauge(metrics.Options{
				Name: gaugePrefix + testCase.name,
				Tags: testCase.tags,
			})
			timer := ff.Timer(metrics.TimerOptions{
				Name:    timerPrefix + testCase.name,
				Tags:    testCase.tags,
				Buckets: testCase.durationBuckets,
			})
			histogram := ff.Histogram(metrics.HistogramOptions{
				Name: histogramPrefix + testCase.name,
				Tags: testCase.tags,
			})

			// register second time, should not panic
			ff.Counter(metrics.Options{
				Name: counterPrefix + testCase.name,
				Tags: testCase.tags,
			})
			ff.Gauge(metrics.Options{
				Name: gaugePrefix + testCase.name,
				Tags: testCase.tags,
			})
			ff.Timer(metrics.TimerOptions{
				Name:    timerPrefix + testCase.name,
				Tags:    testCase.tags,
				Buckets: testCase.durationBuckets,
			})
			ff.Histogram(metrics.HistogramOptions{
				Name: histogramPrefix + testCase.name,
				Tags: testCase.tags,
			})

			counter.Inc(42)
			gauge.Update(42)
			timer.Record(42 * time.Millisecond)
			histogram.Record(42.42)

			assertExpvar(t, fmt.Sprintf(testCase.fullName, counterPrefix), testCase.expectedCounter)
			assertExpvar(t, fmt.Sprintf(testCase.fullName, gaugePrefix), "42")
			assertExpvar(t, fmt.Sprintf(testCase.fullName, timerPrefix)+"_ns", "42000000")
			assertExpvar(t, fmt.Sprintf(testCase.fullName, histogramPrefix), "42.42")
		})
	}
}

func assertExpvar(t *testing.T, fullName string, value string) {
	var found expvar.KeyValue
	expvar.Do(func(kv expvar.KeyValue) {
		if kv.Key == fullName {
			found = kv
		}
	})
	if !assert.Equal(t, fullName, found.Key) {
		expvar.Do(func(kv expvar.KeyValue) {
			if strings.HasPrefix(kv.Key, prefix) {
				t.Log(kv)
			}
		})
		return
	}
	assert.Equal(t, value, found.Value.String(), fullName)
}
