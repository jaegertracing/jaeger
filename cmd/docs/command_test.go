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

package docs

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand(t *testing.T) {
	tests := []struct{
		file string
		flag string
		err string
	}{
		{file: "docs.md"},
		{file: "docs.1", flag: "--format=man"},
		{file: "docs.rst", flag: "--format=rst"},
		{flag: "--format=foo", err: "undefined value of format, possible values are: [md man rst]"},
	}
	for _, test := range tests {
		v := viper.New()
		cmd := Command(v)
		cmd.ParseFlags([]string{test.flag})
		err := cmd.Execute()
		if err == nil {
			f, err := ioutil.ReadFile(test.file)
			require.NoError(t, err)
			assert.True(t, strings.Contains(string(f), "documentation"))
		} else {
			assert.Equal(t, test.err, err.Error())
		}
	}
}

func Test(t *testing.T) {
	v := viper.New()
	cmd := Command(v)
	cmd.RunE(cmd, []string{})
	f, err := ioutil.ReadFile("docs.md")
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(f), "documentation"))
}
