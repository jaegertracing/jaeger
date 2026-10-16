// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startStopTestBinary(t *testing.T, ignoreSignal bool) *Binary {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	readyFile := filepath.Join(t.TempDir(), "ready")
	b := &Binary{Name: "test-child", Cmd: *exec.Command(executable, "-test.run=^TestBinaryStopHelperProcess$")}
	b.Env = append(os.Environ(), "JAEGER_STOP_HELPER="+readyFile)
	if ignoreSignal {
		b.Env = append(b.Env, "JAEGER_STOP_IGNORE=1")
	}
	prepareBinaryCommand(&b.Cmd)
	require.NoError(t, b.Cmd.Start())
	t.Cleanup(func() {
		if b.ProcessState == nil {
			_ = b.Process.Kill()
			b.ProcessState, _ = b.Process.Wait()
		}
	})
	require.Eventually(t, func() bool {
		_, err := os.Stat(readyFile)
		return err == nil
	}, 10*time.Second, 10*time.Millisecond, "child did not install its signal handler")
	return b
}

func TestBinaryStopHelperProcess(t *testing.T) {
	readyFile := os.Getenv("JAEGER_STOP_HELPER")
	if readyFile == "" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	require.NoError(t, os.WriteFile(readyFile, nil, 0o600))
	for range signals {
		if os.Getenv("JAEGER_STOP_IGNORE") == "" {
			return
		}
	}
}

func TestBinaryStop_Idempotent(t *testing.T) {
	b := startStopTestBinary(t, false)

	b.Stop(t)
	b.Stop(t) // second call must not fail after the process has been reaped
	_, err := b.Process.Wait()
	assert.Error(t, err, "process should have already been waited on by Stop()")
}

// A process that terminates on SIGTERM must not wait out the escalation timeout.
func TestBinaryStop_GracefulOnSigterm(t *testing.T) {
	b := startStopTestBinary(t, false)
	b.ShutdownTimeout = time.Minute

	start := time.Now()
	b.Stop(t)
	assert.Less(t, time.Since(start), 10*time.Second,
		"Stop should return as soon as the process exits, not wait out ShutdownTimeout")
	assert.True(t, b.ProcessState.Success(), "child must handle the shutdown signal and exit normally")
}

// A process that ignores SIGTERM must still be killed: a kill is a kill.
func TestBinaryStop_EscalatesToKill(t *testing.T) {
	b := startStopTestBinary(t, true)
	b.ShutdownTimeout = 100 * time.Millisecond

	start := time.Now()
	b.Stop(t)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond,
		"Stop should have waited out ShutdownTimeout before escalating")
	assert.Less(t, elapsed, 30*time.Second, "SIGKILL escalation should not hang")
	assert.False(t, b.ProcessState.Success(), "the uncooperative child must be killed")

	// The process was reaped by Stop, so a second Wait must fail.
	_, err := b.Process.Wait()
	assert.Error(t, err, "process should have been killed and reaped by Stop()")
}

// The default applies when ShutdownTimeout is left unset, which is how every
// caller in this package constructs a Binary.
func TestBinaryStop_DefaultShutdownTimeout(t *testing.T) {
	b := startStopTestBinary(t, false)
	require.Zero(t, b.ShutdownTimeout, "this test asserts the unset-field path")

	b.Stop(t)
	assert.Equal(t, time.Minute, defaultShutdownTimeout)
}
