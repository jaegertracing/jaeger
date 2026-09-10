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

func lookPathOrSkip(t *testing.T, name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s binary not found; this test requires a POSIX environment", name)
	}
	return path
}

func TestBinaryStop_Idempotent(t *testing.T) {
	sleepPath := lookPathOrSkip(t, "sleep")

	b := &Binary{
		Name: "test-sleep",
		Cmd: exec.Cmd{
			Path: sleepPath,
			Args: []string{"sleep", "100"},
		},
	}
	require.NoError(t, b.Cmd.Start())

	b.Stop(t)
	b.Stop(t) // second call must not fail the test (signalling returns os.ErrProcessDone)
	_, err := b.Process.Wait()
	assert.Error(t, err, "process should have already been waited on by Stop()")
}

// A process that terminates on SIGTERM must not wait out the escalation timeout.
func TestBinaryStop_GracefulOnSigterm(t *testing.T) {
	sleepPath := lookPathOrSkip(t, "sleep")

	b := &Binary{
		Name: "test-sleep",
		// Long enough that returning promptly can only mean SIGTERM was honored.
		ShutdownTimeout: time.Minute,
		Cmd: exec.Cmd{
			Path: sleepPath,
			Args: []string{"sleep", "100"},
		},
	}
	require.NoError(t, b.Cmd.Start())

	start := time.Now()
	b.Stop(t)
	assert.Less(t, time.Since(start), 10*time.Second,
		"Stop should return as soon as the process exits, not wait out ShutdownTimeout")
}

// A process that ignores SIGTERM must still be killed: a kill is a kill.
func TestBinaryStop_EscalatesToKill(t *testing.T) {
	shPath := lookPathOrSkip(t, "sh")

	// The shell touches this file after installing the trap. Without waiting for
	// it, Stop can signal before `trap` has run, and the default SIGTERM action
	// terminates the shell — making the test pass for the wrong reason.
	readyFile := filepath.Join(t.TempDir(), "trap-installed")

	b := &Binary{
		Name:            "test-sigterm-ignoring",
		ShutdownTimeout: 100 * time.Millisecond,
		Cmd: exec.Cmd{
			Path: shPath,
			// `trap "" TERM` makes SIGTERM ignored, so only SIGKILL can stop this.
			Args: []string{"sh", "-c", `trap "" TERM; touch "$0"; sleep 10`, readyFile},
		},
	}
	require.NoError(t, b.Cmd.Start())
	require.Eventually(t, func() bool {
		_, err := os.Stat(readyFile)
		return err == nil
	}, 10*time.Second, 10*time.Millisecond, "shell did not install its SIGTERM trap")

	start := time.Now()
	b.Stop(t)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond,
		"Stop should have waited out ShutdownTimeout before escalating")
	assert.Less(t, elapsed, 30*time.Second, "SIGKILL escalation should not hang")

	// The process was reaped by Stop, so a second Wait must fail.
	_, err := b.Process.Wait()
	assert.Error(t, err, "process should have been killed and reaped by Stop()")
}

// The default applies when ShutdownTimeout is left unset, which is how every
// caller in this package constructs a Binary.
func TestBinaryStop_DefaultShutdownTimeout(t *testing.T) {
	sleepPath := lookPathOrSkip(t, "sleep")

	b := &Binary{
		Name: "test-sleep",
		Cmd: exec.Cmd{
			Path: sleepPath,
			Args: []string{"sleep", "100"},
		},
	}
	require.NoError(t, b.Cmd.Start())
	require.Zero(t, b.ShutdownTimeout, "this test asserts the unset-field path")

	b.Stop(t)
	assert.Equal(t, time.Minute, defaultShutdownTimeout)
}
