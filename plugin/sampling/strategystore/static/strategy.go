// Copyright (c) 2018 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package static

// strategy defines a sampling strategy. Type can be "probabilistic" or "ratelimiting"
// and Param will represent "sampling probability" and "max traces per second" respectively.
type strategy struct {
	Type                string               `json:"type"`
	Param               float64              `json:"param"`
	OperationStrategies []*operationStrategy `json:"operation_strategies"`
}

// operationStrategy defines an operation specific sampling strategy.
type operationStrategy struct {
	Operation string `json:"operation"`
	strategy
}

// serviceStrategy defines a service specific sampling strategy.
type serviceStrategy struct {
	Service string `json:"service"`
	strategy
}

// strategies holds a default sampling strategy and service specific sampling strategies.
type strategies struct {
	DefaultStrategy   *strategy          `json:"default_strategy"`
	ServiceStrategies []*serviceStrategy `json:"service_strategies"`
}
