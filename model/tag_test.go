package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

func TestSortTags(t *testing.T) {
	input := []model.Tag{
		model.Tag(model.String("x", "z")),
		model.Tag(model.String("x", "y")),
		model.Tag(model.Int64("a", 2)),
		model.Tag(model.Int64("a", 1)),
		model.Tag(model.Float64("x", 2)),
		model.Tag(model.Float64("x", 1)),
		model.Tag(model.Bool("x", true)),
		model.Tag(model.Bool("x", false)),
	}
	expected := []model.Tag{
		model.Tag(model.Int64("a", 1)),
		model.Tag(model.Int64("a", 2)),
		model.Tag(model.String("x", "y")),
		model.Tag(model.String("x", "z")),
		model.Tag(model.Bool("x", false)),
		model.Tag(model.Bool("x", true)),
		model.Tag(model.Float64("x", 1)),
		model.Tag(model.Float64("x", 2)),
	}
	model.SortTags(input)
	assert.Equal(t, expected, input)
}
