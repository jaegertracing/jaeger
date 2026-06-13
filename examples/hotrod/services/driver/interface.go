// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
)

// Driver describes a driver and the current car location.
type Driver struct {
	DriverID string
	Location string
}

// Interface exposed by the Driver service.
type Interface interface {
	FindNearest(ctx context.Context, location string) ([]Driver, error)
}
