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
	"github.com/uber/jaeger-lib/metrics"

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

// MultiReporter provides serial span emission to one or more reporters.  If
// more than one expensive reporter are needed, one or more of them should be
// wrapped and hidden behind a channel.
type MultiReporter []Reporter

// BatchMetrics provides a common set of metrics that should be reported by reporters
type BatchMetrics struct {
	// Number of successful batch submissions to collector
	BatchesSubmitted metrics.Counter `metric:"batches.submitted"`

	// Number of failed batch submissions to collector
	BatchesFailures metrics.Counter `metric:"batches.failures"`

	// Number of spans in a batch submitted to collector
	BatchSize metrics.Gauge `metric:"batch_size"`

	// Number of successful span submissions to collector
	SpansSubmitted metrics.Counter `metric:"spans.submitted"`

	// Number of failed span submissions to collector
	SpansFailures metrics.Counter `metric:"spans.failures"`
}

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
