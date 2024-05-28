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

package expvar

import (
	"testing"
	"time"
)

func TestCounter(t *testing.T) {
	counter := NewCounter("xyz_counter")
	counter.Inc(123)
	assertExpvar(t, "xyz_counter", "123")
}

func TestGauge(t *testing.T) {
	gauge := NewGauge("xyz_gauge")
	gauge.Update(123)
	assertExpvar(t, "xyz_gauge", "123")
}

func TestTimer(t *testing.T) {
	timer := NewTimer("xyz_timer")
	timer.Record(100*time.Millisecond + 500*time.Microsecond) // 100.5 milliseconds
	assertExpvar(t, "xyz_timer_ns", "100500000")              // 100500000 nanoseconds
}

func TestHistogram(t *testing.T) {
	histogram := NewHistogram("xyz_histogram")
	histogram.Record(3.1415)
	assertExpvar(t, "xyz_histogram", "3.1415")
}
