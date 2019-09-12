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

package common_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/common"
	"github.com/jaegertracing/jaeger/model"
)

func TestAddProcessTagsToAllParts(t *testing.T) {
	assert := assert.New(t)

	process := &model.Process{}

	spans := []*model.Span{
		{
			Process: &model.Process{},
		},
	}

	tags := map[string]string{
		"a": "b",
	}

	spans2, process2 := common.AddProcessTags(spans, process, []model.KeyValue{})
	assert.Equal(process, process2)
	assert.Equal(spans, spans2)

	_, process2 = common.AddProcessTags(spans, process, model.KeyValueFromMap(tags))
	assert.Equal("b", process2.Tags[0].VStr)
}
