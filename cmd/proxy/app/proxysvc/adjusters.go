// Copyright (c) 2019 The Jaeger Authors.
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

package proxysvc

import (
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
)

// StandardAdjusters is a list of model adjusters applied by the proxy service
// before returning the data to the API clients.
func StandardAdjusters(maxClockSkewAdjust time.Duration, pta ProcessTagAdjuster) []adjuster.Adjuster {
	return []adjuster.Adjuster{
		&pta,
		adjuster.SpanIDDeduper(),
		// adjuster.ClockSkew(maxClockSkewAdjust),
		adjuster.IPTagAdjuster(),
		adjuster.SortLogFields(),
		adjuster.SpanReferences(),
	}
}

func CustomAdjusters(pta ProcessTagAdjuster) []adjuster.Adjuster {
	return []adjuster.Adjuster{
		&pta,
	}
}

type ProcessTagAdjuster struct {
	Mapped map[string]string
	Static model.KeyValues
}

func NewProcessTagAdjuster(mapped, static map[string]string) ProcessTagAdjuster {
	var staticKV model.KeyValues
	for k, v := range static {
		staticKV = append(staticKV, model.String(k, v))
	}

	return ProcessTagAdjuster{
		Mapped: mapped,
		Static: staticKV,
	}
}

func (a *ProcessTagAdjuster) Adjust(trace *model.Trace) (*model.Trace, error) {
	for i, span := range trace.Spans {
		for _, processTag := range span.Process.Tags {
			if newKey, ok := a.Mapped[processTag.Key]; ok {
				newTag := processTag
				newTag.Key = newKey
				span.Process.Tags = append(span.Process.Tags, newTag)
			}
		}
		trace.Spans[i].Process.Tags = append(span.Process.Tags, a.Static...)
	}

	return trace, nil
}
