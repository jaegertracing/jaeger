// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignalBinaryProcessInvalidPID(t *testing.T) {
	for _, pid := range []int{-1, 0} {
		require.ErrorContains(t, signalBinaryProcess(&os.Process{Pid: pid}), "invalid process ID")
	}
}
