package metrics

import (
	"time"
)

// Timer tracks how long an event has occurred.
type Timer interface {
	// Records the time passed in.
	Record(time.Duration)
}

// NullTimer timer that does nothing
var NullTimer Timer = nullTimer{}

type nullTimer struct{}

func (nullTimer) Record(time.Duration) {}

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
