// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"github.com/jaegertracing/jaeger/internal/storage/v1/spanstore"
)

func getUniqueOperationNames(operations []spanstore.Operation) []string {
	// only return unique operation names
	set := make(map[string]struct{})
	for _, operation := range operations {
		set[operation.Name] = struct{}{}
	}
	var operationNames []string
	for operation := range set {
		operationNames = append(operationNames, operation)
	}
	return operationNames
}
