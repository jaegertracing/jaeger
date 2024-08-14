// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package processors

// Processor processes metrics in multiple formats
type Processor interface {
	Serve()
	Stop()
}
