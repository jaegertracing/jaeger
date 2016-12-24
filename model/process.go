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

import (
	"encoding/binary"
	"hash/fnv"
)

// Process describes an instance of an application or service that emits tracing data.
type Process struct {
	ServiceName string    `json:"serviceName"`
	Tags        KeyValues `json:"tags,omitempty"`
}

// NewProcess creates a new Process for given serviceName and tags.
// The tags are sorted in place and kept in the the same array/slice,
// in order to store the Process in a canonical form that is relied
// upon by the Equal and Hash functions.
func NewProcess(serviceName string, tags []KeyValue) *Process {
	typedTags := KeyValues(tags)
	typedTags.Sort()
	return &Process{ServiceName: serviceName, Tags: typedTags}
}

// Equal compares KeyValue object with another KeyValue.
func (p *Process) Equal(other *Process) bool {
	if p.ServiceName != other.ServiceName {
		return false
	}
	return p.Tags.Equal(other.Tags)
}

// Hash computes a hash code for the process.
func (p *Process) Hash() uint64 {
	h := fnv.New64a()
	// ignore errors because FNV-1a never returns any
	_, _ = h.Write([]byte(p.ServiceName))
	for i := range p.Tags {
		tag := &p.Tags[i]
		h.Write([]byte(tag.Key))
		binary.Write(h, binary.BigEndian, uint16(tag.VType))
		h.Write([]byte(tag.VStr))
		h.Write(tag.VBlob)
		binary.Write(h, binary.BigEndian, uint64(tag.VNum))
	}
	return h.Sum64()
}
