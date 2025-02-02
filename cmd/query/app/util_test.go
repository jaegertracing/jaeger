// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/storage/v1/spanstore"
)

func Test_getUniqueOperationNames(t *testing.T) {
	assert.Equal(t, []string(nil), getUniqueOperationNames(nil))

	operations := []spanstore.Operation{
		{Name: "operation1", SpanKind: "server"},
		{Name: "operation1", SpanKind: "client"},
		{Name: "operation2"},
	}

	expNames := []string{"operation1", "operation2"}
	names := getUniqueOperationNames(operations)
	assert.Len(t, names, 2)
	assert.ElementsMatch(t, expNames, names)
}
