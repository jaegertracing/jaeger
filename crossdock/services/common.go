// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"fmt"
)

func getTracerServiceName(service string) string {
	return fmt.Sprintf("crossdock-%s", service)
}
