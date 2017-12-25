// Copyright (c) 2017 Uber Technologies, Inc.
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

package adjuster

import (
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/multierror"
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
	var errors []error
	for _, adjuster := range c.adjusters {
		var err error
		trace, err = adjuster.Adjust(trace)
		if err != nil {
			if c.failFast {
				return trace, err
			}
			errors = append(errors, err)
		}
	}
	return trace, multierror.Wrap(errors)
}
