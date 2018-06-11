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

package adjuster

import (
	"bytes"
	"encoding/binary"
	"strconv"

	"github.com/jaegertracing/jaeger/model"
)

var ipTagsToCorrect = map[string]struct{}{
	"ip":        {},
	"peer.ipv4": {},
}

// IPTagAdjuster returns an adjuster that replaces numeric "ip" tags,
// which usually contain IPv4 packed into uint32, with their string
// representation (e.g. "8.8.8.8"").
func IPTagAdjuster() Adjuster {

	adjustTags := func(tags model.KeyValues) {
		for i, tag := range tags {
			var value uint32
			switch tag.VType {
				case model.Int64Type:
					value = uint32(tag.Int64())
				case model.Float64Type:
					value = uint32(tag.Float64())
				default:
					continue
			}
			if _, ok := ipTagsToCorrect[tag.Key]; !ok {
				continue
			}
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], value)
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
			model.KeyValues(span.Process.Tags).Sort()
		}
		return trace, nil
	})
}
