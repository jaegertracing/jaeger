// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBinaryStop(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep binary not found; this test requires a POSIX environment")
	}

	b := &Binary{
		Name: "test-sleep",
		Cmd: exec.Cmd{
			Path: sleepPath,
			Args: []string{"sleep", "100"},
		},
	}
	require.NoError(t, b.Cmd.Start())

	b.Stop(t)

	_, err = b.Process.Wait()
	assert.Error(t, err, "process should have already been waited on by Stop()")
}

func TestBinaryStop_Idempotent(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep binary not found; this test requires a POSIX environment")
	}

	b := &Binary{
		Name: "test-sleep",
		Cmd: exec.Cmd{
			Path: sleepPath,
			Args: []string{"sleep", "100"},
		},
	}
	require.NoError(t, b.Cmd.Start())

	b.Stop(t)
	b.Stop(t) // second call must not fail the test (Kill returns os.ErrProcessDone)
	_, err = b.Process.Wait()
	assert.Error(t, err, "process should have already been waited on by Stop()")
}
