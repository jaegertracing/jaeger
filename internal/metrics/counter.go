// Copyright (c) 2022 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metrics

// Counter tracks the number of times an event has occurred
type Counter interface {
	// Inc adds the given value to the counter.
	Inc(int64)
}

// NullCounter counter that does nothing
var NullCounter Counter = nullCounter{}

type nullCounter struct{}

func (nullCounter) Inc(int64) {}
