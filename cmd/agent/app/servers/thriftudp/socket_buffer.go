// Copyright (c) 2020 The Jaeger Authors.
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

//go:build !windows
// +build !windows

package thriftudp

import (
	"fmt"
	"net"
	"syscall"
)

func setSocketBuffer(conn *net.UDPConn, bufferSize int) error {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return fmt.Errorf("failed to get raw connection: %w", err)
	}

	var syscallErr error
	controlErr := rawConn.Control(func(fd uintptr) {
		syscallErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, bufferSize)
	})
	if controlErr != nil {
		return fmt.Errorf("rawconn control failed: %w", controlErr)
	}
	if syscallErr != nil {
		return fmt.Errorf("syscall failed: %w", syscallErr)
	}

	return nil
}
