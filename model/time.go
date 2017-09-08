// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
