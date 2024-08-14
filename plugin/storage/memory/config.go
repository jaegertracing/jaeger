// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

// Configuration describes the options to customize the storage behavior
type Configuration struct {
	MaxTraces int `mapstructure:"max_traces"`
}
