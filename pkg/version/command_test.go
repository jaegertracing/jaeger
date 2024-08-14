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

package version

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommand(t *testing.T) {
	commitSHA = "foobar"
	latestVersion = "v1.2.3"
	date = "2024-01-04"
	cmd := Command()

	var b bytes.Buffer
	cmd.SetOut(&b)
	err := cmd.Execute()
	require.NoError(t, err)
	out, err := io.ReadAll(&b)
	require.NoError(t, err)
	expectedCommandOutput := `{"gitCommit":"foobar","gitVersion":"v1.2.3","buildDate":"2024-01-04"}`
	assert.Equal(t, expectedCommandOutput, string(out))
}
