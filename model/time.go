// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/model/v1"
)

// EpochMicrosecondsAsTime converts microseconds since epoch to time.Time value.
var EpochMicrosecondsAsTime = modelv1.EpochMicrosecondsAsTime

// TimeAsEpochMicroseconds converts time.Time to microseconds since epoch,
// which is the format the StartTime field is stored in the Span.
var TimeAsEpochMicroseconds = modelv1.TimeAsEpochMicroseconds

// MicrosecondsAsDuration converts duration in microseconds to time.Duration value.
var MicrosecondsAsDuration = modelv1.MicrosecondsAsDuration

// DurationAsMicroseconds converts time.Duration to microseconds,
// which is the format the Duration field is stored in the Span.
var DurationAsMicroseconds = modelv1.DurationAsMicroseconds
