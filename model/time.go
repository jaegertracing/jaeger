// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"time"

	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/model/v1"
)

// EpochMicrosecondsAsTime converts microseconds since epoch to time.Time value.
func EpochMicrosecondsAsTime(ts uint64) time.Time {
	return jaegerIdlModel.EpochMicrosecondsAsTime(ts)
}

// TimeAsEpochMicroseconds converts time.Time to microseconds since epoch,
// which is the format the StartTime field is stored in the Span.
func TimeAsEpochMicroseconds(t time.Time) uint64 {
	return jaegerIdlModel.TimeAsEpochMicroseconds(t)
}

// MicrosecondsAsDuration converts duration in microseconds to time.Duration value.
func MicrosecondsAsDuration(v uint64) time.Duration {
	return jaegerIdlModel.MicrosecondsAsDuration(v)
}

// DurationAsMicroseconds converts time.Duration to microseconds,
// which is the format the Duration field is stored in the Span.
func DurationAsMicroseconds(d time.Duration) uint64 {
	return jaegerIdlModel.DurationAsMicroseconds(d)
}
