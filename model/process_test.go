package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

func TestNewProcess(t *testing.T) {
	p1 := model.NewProcess("s1", []model.Tag{
		model.Tag(model.String("x", "y")),
		model.Tag(model.Int64("a", 1)),
	})
	p2 := model.NewProcess("s1", []model.Tag{
		model.Tag(model.Int64("a", 1)),
		model.Tag(model.String("x", "y")),
	})
	assert.Equal(t, p1, p2)
}
