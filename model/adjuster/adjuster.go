// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"github.com/jaegertracing/jaeger/model"
)

// Adjuster applies certain modifications to a Trace object.
// It returns adjusted Trace, which can be the same Trace updated in place.
// If it detects a problem with the trace that prevents it from applying
// adjustments, it must still return the original trace.
type Adjuster interface {
	Adjust(trace *model.Trace) *model.Trace
}

// Func wraps a function of appropriate signature and makes an Adjuster from it.
type Func func(trace *model.Trace) *model.Trace

// Adjust implements Adjuster interface.
func (f Func) Adjust(trace *model.Trace) *model.Trace {
	return f(trace)
}

// Sequence creates an adjuster that combines a series of adjusters
// applied in order.
func Sequence(adjusters ...Adjuster) Adjuster {
	return sequence{adjusters: adjusters}
}

type sequence struct {
	adjusters []Adjuster
}

func (c sequence) Adjust(trace *model.Trace) *model.Trace {
	for _, adjuster := range c.adjusters {
		trace = adjuster.Adjust(trace)
	}
	return trace
}
