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

package adjuster

import (
	"bytes"
	"encoding/binary"
	"strconv"

	"github.com/uber/jaeger/model"
)

var ipTagsToCorrect = map[string]bool{
	"ip":        true,
	"peer.ipv4": true,
}

// IPTagAdjuster returns an adjuster that replaces numeric "ip" tags,
// which usually contain IPv4 packed into uint32, with their string
// representation (e.g. "8.8.8.8"").
func IPTagAdjuster() Adjuster {

	adjustTags := func(tags model.KeyValues) {
		for i, tag := range tags {
			if tag.VType != model.Int64Type {
				continue
			}
			if _, ok := ipTagsToCorrect[tag.Key]; !ok {
				continue
			}
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], uint32(tag.VNum))
			var sBuf bytes.Buffer
			for i, b := range buf {
				if i > 0 {
					sBuf.WriteRune('.')
				}
				sBuf.WriteString(strconv.FormatUint(uint64(b), 10))
			}
			tags[i] = model.String(tag.Key, sBuf.String())
		}
	}

	return Func(func(trace *model.Trace) (*model.Trace, error) {
		for _, span := range trace.Spans {
			adjustTags(span.Tags)
			adjustTags(span.Process.Tags)
			span.Process.Tags.Sort()
		}
		return trace, nil
	})
}
