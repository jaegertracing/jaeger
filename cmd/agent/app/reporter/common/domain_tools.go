// Copyright (c) 2019 The Jaeger Authors.
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

package common

import "github.com/jaegertracing/jaeger/model"

// AddProcessTags appends jaeger tags for the agent to every span it sends to the collector.
func AddProcessTags(spans []*model.Span, process *model.Process, agentTags []model.KeyValue) ([]*model.Span, *model.Process) {
	if len(agentTags) == 0 {
		return spans, process
	}
	if process != nil {
		process.Tags = append(process.Tags, agentTags...)
	}
	for _, span := range spans {
		if span.Process != nil {
			span.Process.Tags = append(span.Process.Tags, agentTags...)
		}
	}
	return spans, process
}
