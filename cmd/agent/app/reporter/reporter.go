// Copyright (c) 2019 The Jaeger Authors.
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

package reporter

import (
	"github.com/jaegertracing/jaeger/pkg/multierror"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// Reporter handles spans received by Processor and forwards them to central
// collectors.
type Reporter interface {
	EmitZipkinBatch(spans []*zipkincore.Span) (err error)
	EmitBatch(batch *jaeger.Batch) (err error)
}

// RetryableError indicates if the error coming from a reporter can be retried for sending
type RetryableError interface {
	IsRetryable() bool
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
