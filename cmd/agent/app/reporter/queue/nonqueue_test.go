package queue

import (
	"fmt"
	"testing"

	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/stretchr/testify/assert"
)

func TestDirectProcessing(t *testing.T) {
	assert := assert.New(t)
	n := NewNonQueue(func(batch *jaeger.Batch) error {
		return fmt.Errorf("Error")
	})

	err := n.Enqueue(&jaeger.Batch{})
	assert.Error(err, "Error")
}
