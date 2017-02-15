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

import "github.com/uber/jaeger/model"

// GetAllUniqueTags creates a list of all unique tags found in a span and process
func GetAllUniqueTags(span *model.Span) []TagInsertion {
	process := span.Process
	allTags := span.Tags
	allTags = append(allTags, process.Tags...)
	for _, log := range span.Logs {
		allTags = append(allTags, log.Fields...)
	}
	allTags.Sort()
	uniqueTags := make([]TagInsertion, 0, len(allTags))
	for i := range allTags {
		if allTags[i].VType == model.BinaryType {
			continue // do not index binary tags
		}
		if i > 0 && allTags[i-1].Equal(&allTags[i]) {
			continue // skip identical tags
		}
		uniqueTags = append(uniqueTags, TagInsertion{
			ServiceName: process.ServiceName,
			TagKey:      allTags[i].Key,
			TagValue:    allTags[i].AsString(),
		})
	}
	return uniqueTags
}
