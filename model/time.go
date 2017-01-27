// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
