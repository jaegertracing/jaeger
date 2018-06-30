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

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestTagInsertionString(t *testing.T) {
	v := TagInsertion{"x", "y", "z"}
	assert.Equal(t, "x:y:z", v.String())
}

func TestTraceIDString(t *testing.T) {
	id := TraceIDFromDomain(model.NewTraceID(1, 1))
	assert.Equal(t, "10000000000000001", id.String())
}
