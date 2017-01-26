package model

import "time"

// EpochMicrosecondsAsTime converts microseconds since epoch to time.Time value.
func EpochMicrosecondsAsTime(ts uint64) time.Time {
	seconds := ts / 1000000
	nanos := 1000 * (ts % 1000000)
	return time.Unix(int64(seconds), int64(nanos))
}

// TimeAsEpochMicroseconds converts time.Time to microseconds since epoch,
// which is the format the StartTime field is stored in the Span.
func TimeAsEpochMicroseconds(t time.Time) uint64 {
	return uint64(t.UnixNano() / 1000)
}

// MicrosecondsAsDuration converts duration in microseconds to time.Duration value.
func MicrosecondsAsDuration(v uint64) time.Duration {
	return time.Duration(v) * time.Microsecond
}

// DurationAsMicroseconds converts time.Duration to microseconds,
// which is the format the Duration field is stored in the Span.
func DurationAsMicroseconds(d time.Duration) uint64 {
	return uint64(d.Nanoseconds() / 1000)
}
