// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package file

// strategy defines a sampling strategy. Type can be "probabilistic" or "ratelimiting"
// and Param will represent "sampling probability" and "max traces per second" respectively.
type strategy struct {
	Type  string  `json:"type"`
	Param float64 `json:"param"`
}

// operationStrategy defines an operation specific sampling strategy.
type operationStrategy struct {
	Operation string `json:"operation"`
	strategy
}

// serviceStrategy defines a service specific sampling strategy.
type serviceStrategy struct {
	Service             string               `json:"service"`
	OperationStrategies []*operationStrategy `json:"operation_strategies"`
	strategy
}

// strategies holds a default sampling strategy and service specific sampling strategies.
type strategies struct {
	DefaultStrategy   *serviceStrategy   `json:"default_strategy"`
	ServiceStrategies []*serviceStrategy `json:"service_strategies"`
}
