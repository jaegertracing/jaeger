// Copyright (c) 2022 The Jaeger Authors.
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

package expvar_test

import (
	"testing"
	"time"

	"github.com/go-kit/kit/metrics/generic"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/metrics/expvar"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

func TestCounter(t *testing.T) {
	kitCounter := generic.NewCounter("abc")
	var counter metrics.Counter = expvar.NewCounter(kitCounter)
	counter.Inc(123)
	assert.EqualValues(t, 123, kitCounter.Value())
}

func TestGauge(t *testing.T) {
	kitGauge := generic.NewGauge("abc")
	var gauge metrics.Gauge = expvar.NewGauge(kitGauge)
	gauge.Update(123)
	assert.EqualValues(t, 123, kitGauge.Value())
}

func TestTimer(t *testing.T) {
	kitHist := generic.NewHistogram("abc", 10)
	var timer metrics.Timer = expvar.NewTimer(kitHist)
	timer.Record(100*time.Millisecond + 500*time.Microsecond) // 100.5 milliseconds
	assert.EqualValues(t, 0.1005, kitHist.Quantile(0.9))
}

func TestHistogram(t *testing.T) {
	kitHist := generic.NewHistogram("abc", 10)
	var histogram metrics.Histogram = expvar.NewHistogram(kitHist)
	histogram.Record(100)
	assert.EqualValues(t, 100, kitHist.Quantile(0.9))
}
