// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	b.Stop(t) // second call must not fail the test (Signal returns os.ErrProcessDone)
	_, err = b.Process.Wait()
	assert.Error(t, err, "process should have already been waited on by Stop()")
}

func TestBinaryStop_SendsSIGTERM(t *testing.T) {
	b, termFile := startTrapShell(t, "test-handles-sigterm", `trap ': > "$TERM_FILE"; exit 0' TERM`)

	b.Stop(t)

	assert.FileExists(t, termFile, "process should have exited through its SIGTERM handler")
}

func TestBinaryStop_KillsProcessIgnoringSIGTERM(t *testing.T) {
	orig := stopTimeout
	stopTimeout = 100 * time.Millisecond
	t.Cleanup(func() { stopTimeout = orig })

	b, termFile := startTrapShell(t, "test-ignores-sigterm", `trap "" TERM`)

	start := time.Now()
	b.Stop(t)

	assert.GreaterOrEqual(t, time.Since(start), stopTimeout,
		"Stop() should have waited out stopTimeout before escalating to SIGKILL")
	assert.NoFileExists(t, termFile, "process should have ignored SIGTERM")
	_, err := b.Process.Wait()
	assert.Error(t, err, "process should have been killed and reaped by Stop()")
}

// startTrapShell starts a shell that installs the given TERM trap and then idles.
// It returns once the trap is in place, since signalling before that would hit the
// default handler instead, and the path the trap writes to on SIGTERM.
func startTrapShell(t *testing.T, name, trap string) (*Binary, string) {
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh binary not found; this test requires a POSIX environment")
	}

	dir := t.TempDir()
	readyFile := filepath.Join(dir, "ready")
	termFile := filepath.Join(dir, "term")
	b := &Binary{
		Name: name,
		Cmd: exec.Cmd{
			Path: shPath,
			Args: []string{"sh", "-c", trap + `; : > "$READY_FILE"; while true; do sleep 0.1; done`},
			Env:  append(os.Environ(), "READY_FILE="+readyFile, "TERM_FILE="+termFile),
		},
	}
	require.NoError(t, b.Cmd.Start())
	require.Eventually(t, func() bool {
		_, err := os.Stat(readyFile)
		return err == nil
	}, 10*time.Second, 10*time.Millisecond, "shell did not install its TERM trap")
	return b, termFile
}
