// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
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

package zipkin

import (
	"context"

	"github.com/apache/thrift/lib/go/thrift"

	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// SerializeThrift is only used in tests.
func SerializeThrift(spans []*zipkincore.Span) []byte {
	ctx := context.Background()
	t := thrift.NewTMemoryBuffer()
	p := thrift.NewTBinaryProtocolConf(t, &thrift.TConfiguration{})
	p.WriteListBegin(ctx, thrift.STRUCT, len(spans))
	for _, s := range spans {
		s.Write(ctx, p)
	}
	p.WriteListEnd(ctx)
	return t.Buffer.Bytes()
}

// DeserializeThrift decodes Thrift bytes to a list of spans.
func DeserializeThrift(b []byte) ([]*zipkincore.Span, error) {
	ctx := context.Background()
	buffer := thrift.NewTMemoryBuffer()
	buffer.Write(b)

	transport := thrift.NewTBinaryProtocolConf(buffer, &thrift.TConfiguration{})
	_, size, err := transport.ReadListBegin(ctx) // Ignore the returned element type
	if err != nil {
		return nil, err
	}

	// We don't depend on the size returned by ReadListBegin to preallocate the array because it
	// sometimes returns a nil error on bad input and provides an unreasonably large int for size
	var spans []*zipkincore.Span
	for i := 0; i < size; i++ {
		zs := &zipkincore.Span{}
		if err = zs.Read(ctx, transport); err != nil {
			return nil, err
		}
		spans = append(spans, zs)
	}

	return spans, nil
}
