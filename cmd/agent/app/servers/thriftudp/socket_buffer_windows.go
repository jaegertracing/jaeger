// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package thriftudp

import (
	"net"
)

// Not supported on windows, so windows version just returns nil
func setSocketBuffer(_ *net.UDPConn, _ int) error {
	return nil
}
