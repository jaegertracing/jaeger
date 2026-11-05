// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package integration

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func prepareBinaryCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
	cmd.SysProcAttr.HideWindow = true
}

func signalBinaryProcess(process *os.Process) error {
	pid := int64(process.Pid)
	if pid <= 0 || pid > math.MaxUint32 {
		return fmt.Errorf("invalid process ID: %d", pid)
	}
	// Windows cannot send SIGTERM through os.Process.Signal. CTRL_BREAK is
	// delivered as os.Interrupt to Go programs and is scoped to this group.
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(pid))
}
