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

package dbmodel

// UniqueTraceIDs is a set of unique dbmodel TraceIDs, implemented via map.
type UniqueTraceIDs map[TraceID]struct{}

// GetUniqueTraceIDs takes a list of spans and returns a map of traceID as string to traceID as byte
// The only reason here we return such a map is because []byte cannot be a key.
func GetUniqueTraceIDs(dbSpans []Span) UniqueTraceIDs {
	retMe := UniqueTraceIDs{}
	for _, v := range dbSpans {
		retMe[v.TraceID] = struct{}{}
	}
	return retMe
}

// IntersectTraceIDs takes a list of UniqueTraceIDs and intersects them.
func IntersectTraceIDs(uniqueTraceIdsList []UniqueTraceIDs) UniqueTraceIDs {
	retMe := UniqueTraceIDs{}
	for key, value := range uniqueTraceIdsList[0] {
		keyExistsInAll := true
		for _, otherTraceIds := range uniqueTraceIdsList[1:] {
			if _, ok := otherTraceIds[key]; !ok {
				keyExistsInAll = false
				break
			}
		}
		if keyExistsInAll {
			retMe[key] = value
		}
	}
	return retMe
}
