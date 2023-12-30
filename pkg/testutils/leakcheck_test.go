package testutils

import (
	"testing"
)

func TestIgnoreGlogFlushDaemonLeak(t *testing.T) {
	opt := IgnoreGlogFlushDaemonLeak()
	if opt == nil {
		t.Errorf("IgnoreGlogFlushDaemonLeak() returned nil, want non-nil goleak.Option")
	}
}
