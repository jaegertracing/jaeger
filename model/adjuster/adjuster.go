// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"errors"

	"github.com/jaegertracing/jaeger/model"
)

// Adjuster applies certain modifications to a Trace object.
// It returns adjusted Trace, which can be the same Trace updated in place.
// If it detects a problem with the trace that prevents it from applying
// adjustments, it must still return the original trace, and the error.
type Adjuster interface {
	Adjust(trace *model.Trace) (*model.Trace, error)
}

// Func wraps a function of appropriate signature and makes an Adjuster from it.
type Func func(trace *model.Trace) (*model.Trace, error)

// Adjust implements Adjuster interface.
func (f Func) Adjust(trace *model.Trace) (*model.Trace, error) {
	return f(trace)
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

func (c sequence) Adjust(trace *model.Trace) (*model.Trace, error) {
	var errs []error
	for _, adjuster := range c.adjusters {
		var err error
		trace, err = adjuster.Adjust(trace)
		if err != nil {
			if c.failFast {
				return trace, err
			}
			errs = append(errs, err)
		}
	}
	return trace, errors.Join(errs...)
}
