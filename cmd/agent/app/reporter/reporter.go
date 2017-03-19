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
