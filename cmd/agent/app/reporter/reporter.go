// Copyright (c) 2017 Uber Technologies, Inc.
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

package reporter

import (
	"github.com/uber/jaeger/pkg/multierror"
	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

// Reporter handles spans received by Processor and forwards them to central
// collectors.
type Reporter interface {
	EmitZipkinBatch(spans []*zipkincore.Span) (err error)
	EmitBatch(batch *jaeger.Batch) (err error)
}

// MultiReporter provides serial span emission to one or more reporters.  If
// more than one expensive reporter are needed, one or more of them should be
// wrapped and hidden behind a channel.
type MultiReporter []Reporter

// NewMultiReporter creates a MultiReporter from the variadic list of passed
// Reporters.
func NewMultiReporter(reps ...Reporter) MultiReporter {
	return reps
}

// EmitZipkinBatch calls each EmitZipkinBatch, returning the first error.
func (mr MultiReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	var errors []error
	for _, rep := range mr {
		if err := rep.EmitZipkinBatch(spans); err != nil {
			errors = append(errors, err)
		}
	}
	return multierror.Wrap(errors)
}

// EmitBatch calls each EmitBatch, returning the first error.
func (mr MultiReporter) EmitBatch(batch *jaeger.Batch) error {
	var errors []error
	for _, rep := range mr {
		if err := rep.EmitBatch(batch); err != nil {
			errors = append(errors, err)
		}
	}
	return multierror.Wrap(errors)
}
