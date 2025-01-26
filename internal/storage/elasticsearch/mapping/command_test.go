// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mapping

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestCommandExecute(t *testing.T) {
	cmd := Command()

	// TempFile to capture output
	tempFile, err := os.CreateTemp("", "command-output-*.txt")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	// Redirect stdout to the TempFile
	oldStdout := os.Stdout
	os.Stdout = tempFile
	defer func() { os.Stdout = oldStdout }()

	err = cmd.ParseFlags([]string{
		"--mapping=jaeger-span",
		"--es-version=7",
		"--shards=5",
		"--replicas=1",
		"--index-prefix=jaeger-index",
		"--use-ilm=false",
		"--ilm-policy-name=jaeger-ilm-policy",
	})
	require.NoError(t, err)
	require.NoError(t, cmd.Execute())

	output, err := os.ReadFile(tempFile.Name())
	require.NoError(t, err)

	var jsonOutput map[string]any
	err = json.Unmarshal(output, &jsonOutput)
	require.NoError(t, err, "Output should be valid JSON")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
