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

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputFormats(t *testing.T) {
	tests := []struct {
		file string
		flag string
		err  string
	}{
		{file: "docs.md"},
		{file: "docs.1", flag: "--format=man"},
		{file: "docs.rst", flag: "--format=rst"},
		{file: "docs.yaml", flag: "--format=yaml"},
		{flag: "--format=foo", err: "undefined value of format, possible values are: [md man rst yaml]"},
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

func TestDocsForParent(t *testing.T) {
	parent := &cobra.Command{
		Use:   "root_command",
		Short: "some description",
	}
	v := viper.New()
	docs := Command(v)
	parent.AddCommand(docs)
	err := docs.RunE(docs, []string{})
	require.NoError(t, err)
	f, err := ioutil.ReadFile("root_command.md")
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(f), "some description"))
}
