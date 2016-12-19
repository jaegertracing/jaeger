package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

func TestTraceFindSpanByID(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			&model.Span{SpanID: model.SpanID(1), OperationName: "x"},
			&model.Span{SpanID: model.SpanID(2), OperationName: "y"},
			&model.Span{SpanID: model.SpanID(1), OperationName: "z"}, // same span ID
		},
	}
	s1 := trace.FindSpanByID(model.SpanID(1))
	assert.NotNil(t, s1)
	assert.Equal(t, "x", s1.OperationName)
	s2 := trace.FindSpanByID(model.SpanID(2))
	assert.NotNil(t, s2)
	assert.Equal(t, "y", s2.OperationName)
	s3 := trace.FindSpanByID(model.SpanID(3))
	assert.Nil(t, s3)
}
