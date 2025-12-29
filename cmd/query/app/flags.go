// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"github.com/jaegertracing/jaeger/cmd/internal/flags"
)

// Re-export types from cmd/internal/flags for backward compatibility
type (
	UIConfig     = flags.UIConfig
	QueryOptions = flags.QueryOptions
)

// DefaultQueryOptions creates the default configuration for QueryOptions.
func DefaultQueryOptions() QueryOptions {
	return flags.DefaultQueryOptions()
}
