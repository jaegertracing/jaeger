// Copyright (c) 2022 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"time"
)

// Timer accumulates observations about how long some operation took,
// and also maintains a historgam of percentiles.
type Timer interface {
	// Records the time passed in.
	Record(time.Duration)
}

// NullTimer timer that does nothing
var NullTimer Timer = nullTimer{}

type nullTimer struct{}

func (nullTimer) Record(time.Duration) {}
