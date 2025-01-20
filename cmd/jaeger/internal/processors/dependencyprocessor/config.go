// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencyprocessor

import (
	"time"

	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

type Config struct {
	// AggregationInterval defines how often the processor aggregates dependencies.
	// This controls the frequency of flushing dependency data to storage.
	// Default dependency aggregation interval: 10 seconds
	AggregationInterval time.Duration `yaml:"aggregation_interval"`
	// InactivityTimeout specifies the duration of inactivity after which a trace
	// is considered complete and ready for dependency aggregation.
	// Default trace completion timeout: 2 seconds of inactivity
	InactivityTimeout time.Duration `yaml:"inactivity_timeout"`
}
