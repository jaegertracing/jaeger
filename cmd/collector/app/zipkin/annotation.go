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
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
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
		if IsCore(anno.Value) && endpoint.GetServiceName() != "" {
			return endpoint.GetServiceName()
		}
	}
	return ""
}
