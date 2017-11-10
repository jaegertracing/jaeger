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

package zipkin

import (
	"testing"

	"github.com/stretchr/testify/assert"

	zc "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
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
