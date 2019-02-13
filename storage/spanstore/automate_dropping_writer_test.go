package spanstore

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/jaegertracing/jaeger/model"
	"github.com/stretchr/testify/assert"
)

type noopWriteSpanStore struct{}

func (n *noopWriteSpanStore) WriteSpan(span *model.Span) error {
	return nil
}

var errIWillAlwaysFail = errors.New("ErrProneWriteSpanStore will always fail")

type errProneWriteSpanStore struct{}

func (e *errProneWriteSpanStore) WriteSpan(span *model.Span) error {
	return errIWillAlwaysFail
}

func TestNewAutomateDroppingWriter_DefaultConfig(t *testing.T) {
	// In default config where the percentage is 0, newAutomateDroppingWriter will not work
	c := NewAutomateDroppingWriter(&noopWriteSpanStore{}, 0)
	assert.NoError(t, c.WriteSpan(nil))
}

func TestAutomateDroppingWriter_WriteSpan(t *testing.T) {
	c := NewAutomateDroppingWriter(&errProneWriteSpanStore{}, 0.4)
	fmt.Println(float64(math.MaxUint64))
	// The TraceID.Low in this span is within the threshold, span will not be written
	span := &model.Span{
		TraceID: model.TraceID{
			Low:  uint64(float64(math.MaxUint64) * 0.1),
			High: math.MaxUint64,
		},
	}
	assert.NoError(t, c.WriteSpan(span))

	// The TraceID.Low in this span is beyond the threshold, span will be written
	span = &model.Span{
		TraceID: model.TraceID{
			Low:  uint64(float64(math.MaxUint64) * 0.6),
			High: math.MaxUint64,
		},
	}
	assert.EqualError(t, c.WriteSpan(span), fmt.Sprintf("%s", errIWillAlwaysFail))
}
