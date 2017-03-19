package testutils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

func TestInMemoryReporter(t *testing.T) {
	r := NewInMemoryReporter()
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
	assert.Len(t, r.ZipkinSpans(), 1)
	assert.Len(t, r.Spans(), 1)
}
