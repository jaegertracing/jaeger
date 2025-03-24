// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
