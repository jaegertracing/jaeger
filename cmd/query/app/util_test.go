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

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func Test_getUniqueOperationNames(t *testing.T) {
	assert.Equal(t, []string(nil), getUniqueOperationNames(nil))

	operations := []spanstore.Operation{
		{Name: "operation1", SpanKind: "server"},
		{Name: "operation1", SpanKind: "client"},
		{Name: "operation2"},
	}

	expNames := []string{"operation1", "operation2"}
	names := getUniqueOperationNames(operations)
	assert.Equal(t, 2, len(names))
	assert.ElementsMatch(t, expNames, names)
}
