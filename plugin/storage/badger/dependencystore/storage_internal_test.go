package dependencystore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestSeekToSpan(t *testing.T) {
	span := seekToSpan(&model.Trace{}, model.SpanID(uint64(1)))
	assert.Nil(t, span)
}
