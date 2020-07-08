// +build windows

package thriftudp

import "syscall"

func setSocketBuffer(fd uintptr, level, opt, value int) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), level, opt, value)
}
