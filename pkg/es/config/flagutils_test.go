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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFlags(t *testing.T) {
	testCase := []struct {
		name                    string
		indexType               IndexType
		expectedNumShardsFlag   string
		expectedNumReplicasFlag string
		expectedPriorityFlag    string
	}{
		{
			name:                    "span-options",
			indexType:               SPAN,
			expectedNumShardsFlag:   "num-shards-span",
			expectedNumReplicasFlag: "num-replicas-span",
			expectedPriorityFlag:    "priority-span-template",
		},
		{
			name:                    "service-options",
			indexType:               SERVICE,
			expectedNumShardsFlag:   "num-shards-service",
			expectedNumReplicasFlag: "num-replicas-service",
			expectedPriorityFlag:    "priority-service-template",
		},
	}

	for _, tt := range testCase {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedNumShardsFlag, getNumShardPrefix(tt.indexType))
			assert.Equal(t, tt.expectedNumReplicasFlag, getNumReplicasPrefix(tt.indexType))
			assert.Equal(t, tt.expectedPriorityFlag, getPriorityTemplate(tt.indexType))
		})
	}
}
