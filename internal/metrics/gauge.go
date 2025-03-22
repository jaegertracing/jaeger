// Copyright (c) 2022 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metrics

// Gauge returns instantaneous measurements of something as an int64 value
type Gauge interface {
	// Update the gauge to the value passed in.
	Update(int64)
}

// NullGauge gauge that does nothing
var NullGauge Gauge = nullGauge{}

type nullGauge struct{}

func (nullGauge) Update(int64) {}
