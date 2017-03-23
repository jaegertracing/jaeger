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
	"strings"

	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

// IsServerCore checks to see if an annotation is a core server annotation
func IsServerCore(anno string) bool {
	return anno == zipkincore.SERVER_SEND || anno == zipkincore.SERVER_RECV
}

// IsClientCore checks to see if an annotation is a core client annotation
func IsClientCore(anno string) bool {
	return anno == zipkincore.CLIENT_SEND || anno == zipkincore.CLIENT_RECV
}

// IsCore checks to see if an annotation is a core annotation
func IsCore(anno string) bool {
	return IsServerCore(anno) || IsClientCore(anno)
}

// FindServiceName finds annotation in Zipkin span that represents emitting service name.
func FindServiceName(span *zipkincore.Span) string {
	for _, anno := range span.Annotations {
		endpoint := anno.GetHost()
		if endpoint == nil {
			continue
		}
		if IsCore(anno.Value) || strings.Index(anno.Value, "haproxy.") == 0 {
			if endpoint.GetServiceName() != "" {
				return endpoint.GetServiceName()
			}
		}
	}
	return ""
}
