package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTracerServiceName(t *testing.T) {
	assert.Equal(t, "crossdock-go", getTracerServiceName("go"))
}
