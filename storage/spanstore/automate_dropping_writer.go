package spanstore

import (
	"math"

	"github.com/jaegertracing/jaeger/model"
)

// AutomateDroppingWriter is a span Writer that tries to tries to drop spans with
// percentage set in config file
type AutomateDroppingWriter struct {
	spanWriter Writer
	threshold  uint64
}

// NewAutomateDroppingWriter creates a AutomateDroppingWriter
func NewAutomateDroppingWriter(spanWriter Writer, percentage float64) *AutomateDroppingWriter {
	threshold := uint64(percentage * float64(math.MaxUint64))
	return &AutomateDroppingWriter{
		spanWriter: spanWriter,
		threshold:  threshold,
	}
}

// WriteSpan calls WriteSpan on wrapped span writer.
func (c *AutomateDroppingWriter) WriteSpan(span *model.Span) error {
	// TraceID consists of two random generated uint64, we use one of them to decide if span will be written
	if c.threshold != 0 && span.TraceID.Low < c.threshold {
		return nil
	}
	return c.spanWriter.WriteSpan(span)
}
