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

package json_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "github.com/jaegertracing/jaeger/model/json"
)

func TestFromFile(t *testing.T) {
	in, err := ioutil.ReadFile("fixture.json")
	require.NoError(t, err)

	trace, err := FromFile("fixture.json")
	require.NoError(t, err)

	out := &bytes.Buffer{}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(&trace)
	require.NoError(t, err)

	if !assert.Equal(t, string(in), string(out.Bytes())) {
		err := ioutil.WriteFile("fixture-actual.json", out.Bytes(), 0644)
		assert.NoError(t, err)
	}
}

func TestFromFileErrors(t *testing.T) {
	_, err := FromFile("invalid-file-name")
	assert.Error(t, err)

	tmpfile, err := ioutil.TempFile("", "invalid.json")
	require.NoError(t, err)

	defer os.Remove(tmpfile.Name()) // clean up

	content := `{bad json}`
	_, err = tmpfile.Write([]byte(content))
	require.NoError(t, err)
	err = tmpfile.Close()
	require.NoError(t, err)

	_, err = FromFile(tmpfile.Name())
	assert.Error(t, err)
}
