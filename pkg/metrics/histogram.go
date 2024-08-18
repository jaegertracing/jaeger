// Copyright (c) 2018 The Jaeger Authors
// SPDX-License-Identifier: Apache-2.0

package metrics

// Histogram that keeps track of a distribution of values.
type Histogram interface {
	// Records the value passed in.
	Record(float64)
}

// NullHistogram that does nothing
var NullHistogram Histogram = nullHistogram{}

type nullHistogram struct{}

func (nullHistogram) Record(float64) {}
