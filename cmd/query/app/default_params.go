// Copyright (c) 2021 The Jaeger Authors.
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

// Contains default parameter values used by handlers when optional request parameters are missing.

package app

import (
	"time"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
)

var (
	defaultDependencyLookbackDuration   = time.Hour * 24
	defaultTraceQueryLookbackDuration   = time.Hour * 24 * 2
	defaultMetricsQueryLookbackDuration = time.Hour
	defaultMetricsQueryStepDuration     = 5 * time.Second
	defaultMetricsQueryRateDuration     = 10 * time.Minute
	defaultMetricsSpanKinds             = []string{metrics.SpanKind_SPAN_KIND_SERVER.String()}
)
