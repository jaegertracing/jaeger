// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"errors"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Adjuster defines an interface for modifying a trace object in place.
// If the adjuster encounters an issue that prevents it from applying
// modifications, it should return an error. The original trace object
// is modified directly and not returned.
type Adjuster interface {
	Adjust(ptrace.Traces) error
}

// // Func is a type alias that wraps a function and makes an Adjuster from it.
// type Func func(traces ptrace.Traces) (ptrace.Traces, error)

// // Adjust implements Adjuster interface for the Func alias.
// func (f Func) Adjust(traces ptrace.Traces) error {
// 	return f(traces)
// }

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
