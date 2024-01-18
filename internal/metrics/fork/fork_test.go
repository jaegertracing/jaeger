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

package fork

import (
	"testing"
	"time"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

var _ metrics.Factory = (*Factory)(nil)

func TestForkFactory(t *testing.T) {
	forkNamespace := "internal"
	forkFactory := metricstest.NewFactory(time.Second)
	defer forkFactory.Stop()
	defaultFactory := metricstest.NewFactory(time.Second)
	defer defaultFactory.Stop()

	// Create factory that will delegate namespaced metrics to forkFactory
	// and add some metrics
	ff := New(forkNamespace, forkFactory, defaultFactory)
	ff.Gauge(metrics.Options{
		Name: "somegauge",
	}).Update(42)
	ff.Counter(metrics.Options{
		Name: "somecounter",
	}).Inc(2)

	// Check that metrics are presented in defaultFactory backend
	defaultFactory.AssertCounterMetrics(t, metricstest.ExpectedMetric{
		Name:  "somecounter",
		Value: 2,
	})
	defaultFactory.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name:  "somegauge",
		Value: 42,
	})

	// Get default namespaced factory
	defaultNamespacedFactory := ff.Namespace(metrics.NSOptions{
		Name: "default",
	})

	// Add some metrics
	defaultNamespacedFactory.Counter(metrics.Options{
		Name: "somenamespacedcounter",
	}).Inc(111)
	defaultNamespacedFactory.Gauge(metrics.Options{
		Name: "somenamespacedgauge",
	}).Update(222)
	defaultNamespacedFactory.Histogram(metrics.HistogramOptions{
		Name: "somenamespacedhist",
	}).Record(1)
	defaultNamespacedFactory.Timer(metrics.TimerOptions{
		Name: "somenamespacedtimer",
	}).Record(time.Millisecond)

	// Check values in default namespaced factory backend
	defaultFactory.AssertCounterMetrics(t, metricstest.ExpectedMetric{
		Name:  "default.somenamespacedcounter",
		Value: 111,
	})
	defaultFactory.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name:  "default.somenamespacedgauge",
		Value: 222,
	})

	// Get factory with forkNamespace and add some metrics
	internalFactory := ff.Namespace(metrics.NSOptions{
		Name: forkNamespace,
	})
	internalFactory.Gauge(metrics.Options{
		Name: "someinternalgauge",
	}).Update(20)
	internalFactory.Counter(metrics.Options{
		Name: "someinternalcounter",
	}).Inc(50)

	// Check that metrics are presented in forkFactory backend
	forkFactory.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name:  "internal.someinternalgauge",
		Value: 20,
	})
	forkFactory.AssertCounterMetrics(t, metricstest.ExpectedMetric{
		Name:  "internal.someinternalcounter",
		Value: 50,
	})
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
