// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/model/v1"
)

const (
	// SampledFlag is the bit set in Flags in order to define a span as a sampled span
	SampledFlag = Flags(1)
	// DebugFlag is the bit set in Flags in order to define a span as a debug span
	DebugFlag = Flags(2)
	// FirehoseFlag is the bit in Flags in order to define a span as a firehose span
	FirehoseFlag = Flags(8)
)

// Flags is a bit map of flags for a span
type Flags = jaegerIdlModel.Flags
