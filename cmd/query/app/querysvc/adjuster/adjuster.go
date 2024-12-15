// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"errors"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Adjuster is an interface for modifying a trace object in place.
// If an issue is encountered that prevents modifications, an error should be returned.
// The caller must ensure that all spans in the ptrace.Traces argument
// belong to the same trace and represent the complete trace.
type Adjuster interface {
	Adjust(ptrace.Traces) error
}

// Sequence creates an adjuster that combines a series of adjusters
// applied in order. Errors from each step are accumulated and returned
// in the end as a single wrapper error. Errors do not interrupt the
// sequence of adapters.
func Sequence(adjusters ...Adjuster) Adjuster {
	return sequence{adjusters: adjusters}
}

// FailFastSequence is similar to Sequence() but returns immediately
// if any adjuster returns an error.
func FailFastSequence(adjusters ...Adjuster) Adjuster {
	return sequence{adjusters: adjusters, failFast: true}
}

type sequence struct {
	adjusters []Adjuster
	failFast  bool
}

func (c sequence) Adjust(traces ptrace.Traces) error {
	var errs []error
	for _, adjuster := range c.adjusters {
		err := adjuster.Adjust(traces)
		if err != nil {
			if c.failFast {
				return err
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
