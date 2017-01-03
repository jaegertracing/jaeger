// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package json_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "github.com/uber/jaeger/model/json"
)

func TestModel(t *testing.T) {
	in, err := ioutil.ReadFile("fixture.json")
	require.NoError(t, err)

	trace, err := FromFile("fixture.json")
	require.NoError(t, err)

	out := &bytes.Buffer{}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(&trace)
	require.NoError(t, err)

	assert.Equal(t, string(in), string(out.Bytes()))
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
