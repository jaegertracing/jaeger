// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package adjuster

import (
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/multierror"
)

// Adjuster applies certain modifications to a Trace object.
// It returns adjusted Trace, which can be the same Trace updated in place.
type Adjuster interface {
	Adjust(trace model.Trace) (model.Trace, error)
}

// Func wraps a function of appropriate signature and makes an Adjuster from it.
type Func func(trace model.Trace) (model.Trace, error)

// Adjust implements Adjuster interface.
func (f Func) Adjust(trace model.Trace) (model.Trace, error) {
	return f(trace)
}

// Sequence creates an adjuster that combines a series of adjusters
// applied in order.
func Sequence(adjusters ...Adjuster) Adjuster {
	return sequence(adjusters)
}

type sequence []Adjuster

func (c sequence) Adjust(trace model.Trace) (model.Trace, error) {
	var errors []error
	for _, adjuster := range c {
		var err error
		trace, err = adjuster.Adjust(trace)
		if err != nil {
			errors = append(errors, err)
		}
	}
	return trace, multierror.Wrap(errors)
}
