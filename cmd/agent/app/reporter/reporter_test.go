package reporter

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"

	"code.uber.internal/infra/jaeger/oss/cmd/agent/app/testutils"
)

func TestMultiReporter(t *testing.T) {
	r1, r2 := testutils.NewInMemoryReporter(), testutils.NewInMemoryReporter()
	r := NewMultiReporter(r1, r2)
	e1 := r.EmitZipkinBatch([]*zipkincore.Span{
		{},
	})
	e2 := r.EmitBatch(&jaeger.Batch{
		Spans: []*jaeger.Span{
			{},
		},
	})
	assert.NoError(t, e1)
	assert.NoError(t, e2)
	assert.Len(t, r1.ZipkinSpans(), 1)
	assert.Len(t, r1.Spans(), 1)
	assert.Len(t, r2.ZipkinSpans(), 1)
	assert.Len(t, r2.Spans(), 1)
}

func TestMultiReporterErrors(t *testing.T) {
	errMsg := "doh!"
	err := errors.New(errMsg)
	r1, r2 := alwaysFailReporter{err: err}, alwaysFailReporter{err: err}
	r := NewMultiReporter(r1, r2)
	e1 := r.EmitZipkinBatch([]*zipkincore.Span{
		{},
	})
	e2 := r.EmitBatch(&jaeger.Batch{
		Spans: []*jaeger.Span{
			{},
		},
	})
	assert.EqualError(t, e1, fmt.Sprintf("[%s, %s]", errMsg, errMsg))
	assert.EqualError(t, e2, fmt.Sprintf("[%s, %s]", errMsg, errMsg))
}

type alwaysFailReporter struct {
	err error
}

func (r alwaysFailReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	return r.err
}

func (r alwaysFailReporter) EmitBatch(batch *jaeger.Batch) error {
	return r.err
}
