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

package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommand(t *testing.T) {
	cmd := Command()

	assert.NotNil(t, cmd, "Command() should return a non-nil *cobra.Command instance")

	expectedShortDescription := "Jaeger backend v2"
	assert.Equal(t, expectedShortDescription, cmd.Short, "Command short description should be '%s'", expectedShortDescription)

	expectedLongDescription := "Jaeger backend v2"
	assert.Equal(t, expectedLongDescription, cmd.Long, "Command long description should be '%s'", expectedLongDescription)

	assert.NotNil(t, cmd.RunE, "Command should have RunE function set")
}
