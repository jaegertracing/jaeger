// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"testing"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
)

func TestTimeConversion(t *testing.T) {
	s := model.TimeAsEpochMicroseconds(time.Unix(100, 2000))
	assert.Equal(t, uint64(100000002), s)
	assert.True(t, time.Unix(100, 2000).Equal(model.EpochMicrosecondsAsTime(s)))
}

func TestDurationConversion(t *testing.T) {
	d := model.DurationAsMicroseconds(12345 * time.Microsecond)
	assert.Equal(t, 12345*time.Microsecond, model.MicrosecondsAsDuration(d))
	assert.Equal(t, uint64(12345), d)
}

func TestTimeZoneUTC(t *testing.T) {
	ts := model.EpochMicrosecondsAsTime(10000100)
	assert.Equal(t, time.UTC, ts.Location())
}
