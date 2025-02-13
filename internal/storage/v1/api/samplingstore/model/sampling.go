// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

// Throughput keeps track of the queries an operation received.
type Throughput struct {
	Service       string
	Operation     string
	Count         int64
	Probabilities map[string]struct{}
}

// ServiceOperationProbabilities contains the sampling probabilities for all operations in a service.
// ie [service][operation] = probability
type ServiceOperationProbabilities map[string]map[string]float64

// ServiceOperationQPS contains the qps for all operations in a service.
// ie [service][operation] = qps
type ServiceOperationQPS map[string]map[string]float64
