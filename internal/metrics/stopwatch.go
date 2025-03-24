// Copyright (c) 2022 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"time"
)

// StartStopwatch begins recording the executing time of an event, returning
// a Stopwatch that should be used to stop the recording the time for
// that event.  Multiple events can be occurring simultaneously each
// represented by different active Stopwatches
func StartStopwatch(timer Timer) Stopwatch {
	return Stopwatch{t: timer, start: time.Now()}
}

// A Stopwatch tracks the execution time of a specific event
type Stopwatch struct {
	t     Timer
	start time.Time
}

// Stop stops executing of the stopwatch and records the amount of elapsed time
func (s Stopwatch) Stop() {
	s.t.Record(s.ElapsedTime())
}

// ElapsedTime returns the amount of elapsed time (in time.Duration)
func (s Stopwatch) ElapsedTime() time.Duration {
	return time.Since(s.start)
}
