// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Adjuster is an interface for modifying a trace object in place.
// If an issue is encountered that prevents modifications, an error should be returned.
// The caller must ensure that all spans in the ptrace.Traces argument
// belong to the same trace and represent the complete trace.
type Adjuster interface {
	Adjust(ptrace.Traces)
}

// Func is a type alias that wraps a function and makes an Adjuster from it.
type Func func(traces ptrace.Traces)

// Adjust implements Adjuster interface for the Func alias.
func (f Func) Adjust(traces ptrace.Traces) {
	f(traces)
}

// Sequence creates an adjuster that combines a series of adjusters
// applied in order. Errors from each step are accumulated and returned
// in the end as a single wrapper error. Errors do not interrupt the
// sequence of adapters.
func Sequence(adjusters ...Adjuster) Adjuster {
	return sequence{adjusters: adjusters}
}

type sequence struct {
	adjusters []Adjuster
}

func (c sequence) Adjust(traces ptrace.Traces) {
	for _, adjuster := range c.adjusters {
		adjuster.Adjust(traces)
	}
}
