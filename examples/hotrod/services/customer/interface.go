// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package customer

import (
	"context"
)

// Customer contains data about a customer.
type Customer struct {
	ID       string
	Name     string
	Location string
}

// Interface exposed by the Customer service.
type Interface interface {
	Get(ctx context.Context, customerID int) (*Customer, error)
}
