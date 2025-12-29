// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net"
	"strconv"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
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

// getPortForAddr returns the port of an endpoint address.
func getPortForAddr(addr net.Addr) (int, error) {
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return -1, err
	}

	parsedPort, err := strconv.Atoi(port)
	if err != nil {
		return -1, err
	}

	return parsedPort, nil
}
