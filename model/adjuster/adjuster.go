// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"github.com/jaegertracing/jaeger/model"
)

// Adjuster is an interface for modifying a trace object in place.
type Adjuster interface {
	Adjust(trace *model.Trace)
}

// Func wraps a function of appropriate signature and makes an Adjuster from it.
type Func func(trace *model.Trace)

// Adjust implements Adjuster interface.
func (f Func) Adjust(trace *model.Trace) {
	f(trace)
}

// Sequence creates an adjuster that combines a series of adjusters
// applied in order.
func Sequence(adjusters ...Adjuster) Adjuster {
	return sequence{adjusters: adjusters}
}

type sequence struct {
	adjusters []Adjuster
}

func (c sequence) Adjust(trace *model.Trace) {
	for _, adjuster := range c.adjusters {
		adjuster.Adjust(trace)
	}
}
