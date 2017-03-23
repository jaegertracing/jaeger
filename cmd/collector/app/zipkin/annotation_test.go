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

package zipkin

import (
	"testing"

	"github.com/stretchr/testify/assert"

	zc "github.com/uber/jaeger/thrift-gen/zipkincore"
)

func TestCoreChecks(t *testing.T) {
	testChunks := []struct {
		anno   string
		server bool
		client bool
		core   bool
	}{
		{"sneh", false, false, false},
		{zc.SERVER_SEND, true, false, true},
		{zc.SERVER_RECV, true, false, true},
		{zc.CLIENT_SEND, false, true, true},
		{zc.CLIENT_RECV, false, true, true},
	}
	for _, chunk := range testChunks {
		assert.EqualValues(t, chunk.core, IsCore(chunk.anno))
		assert.EqualValues(t, chunk.server, IsServerCore(chunk.anno))
		assert.EqualValues(t, chunk.client, IsClientCore(chunk.anno))
	}
}

func TestFindServiceName(t *testing.T) {
	span := &zc.Span{}
	assert.Equal(t, "", FindServiceName(span), "no annotations")

	anno := &zc.Annotation{
		Value: "cs",
	}
	span.Annotations = []*zc.Annotation{anno}
	assert.Equal(t, "", FindServiceName(span), "no Host in the annotation")

	anno = &zc.Annotation{
		Value: "cs",
		Host: &zc.Endpoint{
			ServiceName: "zoidberg",
		},
	}
	span.Annotations = append(span.Annotations, anno)
	assert.Equal(t, "zoidberg", FindServiceName(span))

	anno = &zc.Annotation{
		Value: "cs",
		Host: &zc.Endpoint{
			ServiceName: "tracegen",
		},
	}
	span.Annotations = append(span.Annotations, anno)
	assert.Equal(t, "zoidberg", FindServiceName(span), "first suitable annotation is picked")

	anno = &zc.Annotation{
		Value: "haproxy.Tr",
		Host: &zc.Endpoint{
			ServiceName: "tracegen",
		},
	}
	span.Annotations = []*zc.Annotation{anno}
	assert.Equal(t, "tracegen", FindServiceName(span), "haproxy annotations also count")

	anno = &zc.Annotation{
		Value: "random event",
		Host: &zc.Endpoint{
			ServiceName: "tracegen",
		},
	}
	span.Annotations = []*zc.Annotation{anno}
	assert.Equal(t, "", FindServiceName(span), "random events do not count")
}
