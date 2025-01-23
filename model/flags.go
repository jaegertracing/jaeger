// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/model/v1"
)

const (
	// SampledFlag is the bit set in Flags in order to define a span as a sampled span
	SampledFlag = jaegerIdlModel.SampledFlag
	// DebugFlag is the bit set in Flags in order to define a span as a debug span
	DebugFlag = jaegerIdlModel.SampledFlag
	// FirehoseFlag is the bit in Flags in order to define a span as a firehose span
	FirehoseFlag = jaegerIdlModel.SampledFlag
)

// Flags is a bit map of flags for a span
type Flags = jaegerIdlModel.Flags
