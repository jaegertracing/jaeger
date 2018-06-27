package spanstore

import (
	"testing"
	"errors"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

var _ Writer = &ChainedWriter{} // check API conformance

type mockWriter struct {
	shouldErr bool
}

func (w *mockWriter) WriteSpan(span *model.Span) error {
	if w.shouldErr {
		return errors.New("")
	}
	return nil
}

func TestChainedWriter(t *testing.T) {
	writer := NewChainedWriter(mockWriter{shouldErr: false})
	assert.NoError(t, writer.WriteSpan(nil))

	writer = NewChainedWriter(mockWriter{shouldErr: false}, mockWriter{shouldErr: true})
	assert.Error(t, writer.WriteSpan(nil))
}
