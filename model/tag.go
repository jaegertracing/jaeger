// Copyright (c) 2016 Uber Technologies, Inc.
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

import "sort"

// Tag is a key-value attribute associated with a Span
type Tag KeyValue

// Tags is a type alias that exposes convenience functions like Sort, FindByKey.
type Tags []Tag

func (tags Tags) Len() int      { return len(tags) }
func (tags Tags) Swap(i, j int) { tags[i], tags[j] = tags[j], tags[i] }
func (tags Tags) Less(i, j int) bool {
	one := KeyValue(tags[i])
	two := KeyValue(tags[j])
	return IsLess(&one, &two)
}

// Sort does in-place sorting of tags by key, then by value type, then by value.
func (tags Tags) Sort() {
	sort.Sort(tags)
}

// FindByKey scans the list of tags searching for the first one with the given key.
// Returns found tag and a boolean flag indicating if the search was successful.
func (tags Tags) FindByKey(key string) (Tag, bool) {
	for _, tag := range tags {
		if tag.Key == key {
			return tag, true
		}
	}
	return Tag{}, false
}
