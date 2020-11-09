// Copyright (c) 2020 The Jaeger Authors.
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

package app

import "github.com/jaegertracing/jaeger/ports"

const (
	// DefaultQueryGRPCHost is the default host for jaeger-query endpoint
	DefaultQueryGRPCHost    = "localhost"
	// DefaultQueryGRPCPort is the default port for jaeger-query endpoint
	DefaultQueryGRPCPort    = ports.QueryHTTP
	// DefaultOutputDir is the default output directory for spans
	DefaultOutputDir        = "/tmp"
	// DefaultHashStandardTags is the default flag for whether to hash standard tags
	DefaultHashStandardTags = true
	// DefaultHashCustomTags is the default flag for whether to hash custom tags
	DefaultHashCustomTags   = false
	// DefaultHashLogs is the default flag for whether to hash logs
	DefaultHashLogs         = false
	// DefaultHashProcess is the default flag for whether to hash process
	DefaultHashProcess      = false
	// DefaultMaxSpansCount is the default value of maximum number of spans
	DefaultMaxSpansCount    = -1
)
