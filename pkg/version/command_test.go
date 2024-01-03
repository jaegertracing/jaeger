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
)

func TestNewCommand(t *testing.T) {
	commitSHA = "foobar"
	latestVersion = "v1.2.3"
	date = "2024-01-04"
	cmd := Command()

	expectedUse := "version"
	assert.Equal(t, expectedUse, cmd.Use, "Command use should be '%s'", expectedUse)

	expectedShortDescription := "Print the version."
	assert.Equal(t, expectedShortDescription, cmd.Short, "Command short description should be '%s'", expectedShortDescription)

	expectedLongDescription := `Print the version and build information.`
	assert.Equal(t, expectedLongDescription, cmd.Long, "Command long description should be '%s'", expectedLongDescription)

	var b bytes.Buffer
	cmd.SetOut(&b)
	cmd.Execute()
	out, err := io.ReadAll(&b)
	if err != nil {
		t.Fatal(err)
	}
	expectedCommandOutput := `{"gitCommit":"foobar","gitVersion":"v1.2.3","buildDate":"2024-01-04"}`
	assert.Equal(t, expectedCommandOutput, string(out), "Command output should be '%s'", expectedCommandOutput)
}
