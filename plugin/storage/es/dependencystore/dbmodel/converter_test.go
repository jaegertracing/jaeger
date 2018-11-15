// Copyright (c) 2018 The Jaeger Authors.
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

	"github.com/jaegertracing/jaeger/model"
)

func TestConvertDependencies(t *testing.T) {
	tests := []struct {
		dLinks []model.DependencyLink
	}{
		{
			dLinks: []model.DependencyLink{{CallCount: 1, Parent: "foo", Child: "bar"}},
		},
		{
			dLinks: []model.DependencyLink{{CallCount: 3, Parent: "foo"}},
		},
		{
			dLinks: []model.DependencyLink{},
		},
		{
			dLinks: nil,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			got := FromDomainDependencies(test.dLinks)
			a := ToDomainDependencies(got)
			assert.Equal(t, test.dLinks, a)
		})
	}
}
