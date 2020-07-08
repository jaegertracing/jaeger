// +build linux darwin

package thriftudp

import "syscall"

func setSocketBuffer(fd uintptr, level, opt, value int) error {
	return syscall.SetsockoptInt(int(fd), level, opt, value)
}
