// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package integration

import (
	"os"
	"os/exec"
	"syscall"
)

func prepareBinaryCommand(_ *exec.Cmd) {}

func signalBinaryProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
