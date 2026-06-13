// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package docs

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
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
			f, err := os.ReadFile(test.file)
			require.NoError(t, err)
			assert.Contains(t, string(f), "documentation")
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
	f, err := os.ReadFile("root_command.md")
	require.NoError(t, err)
	assert.Contains(t, string(f), "some description")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
