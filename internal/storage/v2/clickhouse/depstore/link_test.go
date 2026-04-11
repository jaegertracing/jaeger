// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

var testModelDeps = []model.DependencyLink{
	{Parent: "serviceA", Child: "serviceB", CallCount: 10},
	{Parent: "serviceB", Child: "serviceC", CallCount: 5},
}

var testDBLinks = dependencyLinks{
	{Parent: "serviceA", Child: "serviceB", CallCount: 10},
	{Parent: "serviceB", Child: "serviceC", CallCount: 5},
}

func TestDependencyLink(t *testing.T) {
	t.Run("fromModel", func(t *testing.T) {
		got := dependencyLinkFromModel(testModelDeps[0])
		assert.Equal(t, testDBLinks[0], got)
	})
	t.Run("toModel", func(t *testing.T) {
		got := testDBLinks[0].toModel()
		assert.Equal(t, testModelDeps[0], got)
	})
	t.Run("roundTrip", func(t *testing.T) {
		got := dependencyLinkFromModel(testModelDeps[0]).toModel()
		assert.Equal(t, testModelDeps[0], got)
	})
}

func TestDependencyLinks(t *testing.T) {
	t.Run("fromModel", func(t *testing.T) {
		got := dependencyLinksFromModel(testModelDeps)
		assert.Equal(t, testDBLinks, got)
	})
	t.Run("toModel", func(t *testing.T) {
		got := testDBLinks.toModel()
		assert.Equal(t, testModelDeps, got)
	})
	t.Run("roundTrip", func(t *testing.T) {
		got := dependencyLinksFromModel(testModelDeps).toModel()
		assert.Equal(t, testModelDeps, got)
	})
}
