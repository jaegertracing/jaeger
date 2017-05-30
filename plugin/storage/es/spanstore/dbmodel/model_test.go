package dbmodel

import (
	"testing"
	"github.com/uber/jaeger/model"
	"github.com/stretchr/testify/assert"
)

func TestTagIDString(t *testing.T) {
	id := TraceIDFromDomain(model.TraceID{High: 1, Low: 1})
	traceID, err := id.TraceIDToDomain()
	if err != nil {
		assert.FailNow(t, "Failed to convert traceID back.")
	}
	assert.Equal(t, "10000000000000001", traceID.String())
}
