// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
