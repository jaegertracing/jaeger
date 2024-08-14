// Copyright (c) 2024 The Jaeger Authors.
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

package dbmodel

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestConvertDependencies(t *testing.T) {
	tests := []struct {
		throughputs []*model.Throughput
	}{
		{
			throughputs: []*model.Throughput{{Service: "service1", Operation: "operation1", Count: 10, Probabilities: map[string]struct{}{"new-srv": {}}}},
		},
		{
			throughputs: []*model.Throughput{},
		},
		{
			throughputs: nil,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			got := FromThroughputs(test.throughputs)
			a := ToThroughputs(got)
			assert.Equal(t, test.throughputs, a)
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
