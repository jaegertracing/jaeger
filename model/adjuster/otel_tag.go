// Copyright (c) 2023 The Jaeger Authors.
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
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/jaegertracing/jaeger/model"
)

var otelLibraryKeys = map[string]struct{}{
	string(semconv.OTelLibraryNameKey):    {},
	string(semconv.OTelLibraryVersionKey): {},
}

func OTelTagAdjuster() Adjuster {
	return Func(func(trace *model.Trace) (*model.Trace, error) {
		for _, span := range trace.Spans {
			filteredTags := make([]model.KeyValue, 0)
			for _, tag := range span.Tags {
				if _, ok := otelLibraryKeys[tag.Key]; !ok {
					filteredTags = append(filteredTags, tag)
					continue
				}
				span.Process.Tags = append(span.Process.Tags, tag)
			}
			span.Tags = filteredTags
			model.KeyValues(span.Process.Tags).Sort()
		}
		return trace, nil
	})
}
