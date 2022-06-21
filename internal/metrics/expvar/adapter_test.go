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

	jlibmetrics "github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

func TestAdapter(t *testing.T) {
	f := newAdapter(jlibmetrics.NullFactory)
	f.Counter(metrics.Options{})
	f.Timer(metrics.TimerOptions{})
	f.Gauge(metrics.Options{})
	f.Histogram(metrics.HistogramOptions{})
	f.Namespace(metrics.NSOptions{})
	f.Unwrap()
}
