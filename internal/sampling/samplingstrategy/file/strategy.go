// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"fmt"
	"math"
)

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

// validate checks that Param is within the range its Type allows. It does not
// reject unknown types, which parseStrategy handles by falling back to the
// default strategy.
func (s *strategy) validate(where string) error {
	switch s.Type {
	case samplerTypeProbabilistic:
		if s.Param < 0 || s.Param > 1 || math.IsNaN(s.Param) {
			return fmt.Errorf("%s: probabilistic sampling param must be between 0 and 1, got %v", where, s.Param)
		}
	case samplerTypeRateLimiting:
		// parseStrategy converts Param to int32; values outside that range
		// wrap or are implementation-defined, so refuse them here.
		if s.Param < 0 || s.Param > math.MaxInt32 || math.IsNaN(s.Param) {
			return fmt.Errorf("%s: ratelimiting sampling param must be between 0 and %d, got %v", where, math.MaxInt32, s.Param)
		}
	default:
		// Unknown types are reported by parseStrategy.
	}
	return nil
}

func (s *serviceStrategy) validate(where string) error {
	if err := s.strategy.validate(where); err != nil {
		return err
	}
	for i, op := range s.OperationStrategies {
		// A JSON null in the list unmarshals to a nil pointer, which
		// parseServiceStrategies would dereference.
		if op == nil {
			return fmt.Errorf("%s: operation strategy %d must not be null", where, i)
		}
		if err := op.validate(fmt.Sprintf("%s, operation %q", where, op.Operation)); err != nil {
			return err
		}
	}
	return nil
}

// validate rejects strategies whose params are outside the range that
// parseStrategy can serve to clients, so that neither startup nor a reload
// replaces a working configuration with an invalid one.
func (s *strategies) validate() error {
	if s == nil {
		return nil
	}
	if s.DefaultStrategy != nil {
		if err := s.DefaultStrategy.validate("default strategy"); err != nil {
			return err
		}
	}
	for i, svc := range s.ServiceStrategies {
		if svc == nil {
			return fmt.Errorf("service strategy %d must not be null", i)
		}
		if err := svc.validate(fmt.Sprintf("service %q", svc.Service)); err != nil {
			return err
		}
	}
	return nil
}
