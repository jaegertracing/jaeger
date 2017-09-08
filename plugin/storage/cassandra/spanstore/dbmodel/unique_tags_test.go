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

package dbmodel

import (
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
)

func TestGetUniqueTags(t *testing.T) {
	expectedTags := getTestUniqueTags()
	uniqueTags := GetAllUniqueTags(getTestJaegerSpan())
	if !assert.EqualValues(t, expectedTags, uniqueTags) {
		for _, diff := range pretty.Diff(expectedTags, uniqueTags) {
			t.Log(diff)
		}
	}
}
