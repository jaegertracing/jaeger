// +build linux darwin

package thriftudp

import (
	"net"
	"syscall"
)

func setSocketBuffer(conn *net.UDPConn, bufferSize int) error {
	file, err := conn.File()
	if err != nil {
		return err
	}

	return syscall.SetsockoptInt(int(file.Fd()), syscall.SOL_SOCKET, syscall.SO_RCVBUF, bufferSize)
}
