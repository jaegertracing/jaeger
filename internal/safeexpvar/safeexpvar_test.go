// Copyright (c) 2024 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
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

package safeexpvar

import (
	"expvar"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestSetInt(t *testing.T) {
	// Test with a new variable
	name := "metrics-test-1"
	value := int64(42)

	SetInt(name, value)

	// Retrieve the variable and check its value
	v := expvar.Get(name)
	assert.NotNil(t, v, "expected variable %s to be created", name)
	expInt, ok := v.(*expvar.Int)
	assert.True(t, ok, "expected variable %s to be of type *expvar.Int", name)
	assert.Equal(t, value, expInt.Value(), "expected variable %s value to be %d", name, value)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
