package testutils

import (
	"go.uber.org/goleak"
)

// IgnoreGlogFlushDaemonLeak returns a goleak.Option that ignores the flushDaemon function
// from the glog package that can cause false positives in leak detection.
// This is necessary because glog starts a goroutine in the background that may not
// be stopped when the test finishes, leading to a detected but expected leak.
func IgnoreGlogFlushDaemonLeak() goleak.Option {
	return goleak.IgnoreTopFunction("github.com/golang/glog.(*fileSink).flushDaemon")
}
