// Copyright (c) 2022 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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
	"strings"
	"time"
)

// Counter is an adapter from go-kit Counter to jaeger-lib Counter
type Counter struct {
	intVar *expvar.Int
}

// NewCounter creates a new Counter
func NewCounter(key string) *Counter {
	return &Counter{intVar: expvar.NewInt(key)}
}

// Inc adds the given value to the counter.
func (c *Counter) Inc(delta int64) {
	c.intVar.Add(delta)
}

// Gauge is an adapter from go-kit Gauge to jaeger-lib Gauge
type Gauge struct {
	intVar *expvar.Int
}

// NewGauge creates a new Gauge
func NewGauge(key string) *Gauge {
	return &Gauge{intVar: expvar.NewInt(key)}
}

// Update the gauge to the value passed in.
func (g *Gauge) Update(value int64) {
	g.intVar.Set(value)
}

// Timer only records the latest value (like a Gauge).
type Timer struct {
	intVar *expvar.Int
}

// NewTimer creates a new Timer.
func NewTimer(key string) *Timer {
	if !strings.HasSuffix(key, "_ns") {
		key += "_ns"
	}
	return &Timer{intVar: expvar.NewInt(key)}
}

// Record saves the time passed in.
func (t *Timer) Record(delta time.Duration) {
	t.intVar.Set(delta.Nanoseconds())
}

// Histogram only records the latest value (like a Gauge).
type Histogram struct {
	floatVar *expvar.Float
}

// NewHistogram creates a new Histogram
func NewHistogram(key string) *Histogram {
	return &Histogram{floatVar: expvar.NewFloat(key)}
}

// Record saves the value passed in.
func (t *Histogram) Record(value float64) {
	t.floatVar.Set(value)
}
