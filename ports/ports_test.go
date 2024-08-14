// Copyright (c) 2020 The Jaeger Authors.
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

package ports

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestPortToHostPort(t *testing.T) {
	assert.Equal(t, ":42", PortToHostPort(42))
}

func TestFormatHostPort(t *testing.T) {
	assert.Equal(t, ":42", FormatHostPort("42"))
	assert.Equal(t, ":831", FormatHostPort(":831"))
	assert.Equal(t, "", FormatHostPort(""))
	assert.Equal(t, "localhost:42", FormatHostPort("localhost:42"))
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
