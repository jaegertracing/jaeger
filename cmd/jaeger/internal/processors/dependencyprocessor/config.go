// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencyprocessor

import (
	"time"

	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

type Config struct {
	AggregationInterval time.Duration `yaml:"aggregation_interval"`
	InactivityTimeout   time.Duration `yaml:"inactivity_timeout"`
	Store               *memory.Store `yaml:"-"`
}

func DefaultConfig() Config {
	return Config{
		AggregationInterval: 5 * time.Second, // Default dependency aggregation interval: 5 seconds
		InactivityTimeout:   2 * time.Second, // Default trace completion timeout: 2 seconds of inactivity
		Store:               memory.NewStore(),
	}
}
