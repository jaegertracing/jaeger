// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net"
	"strconv"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func getUniqueOperationNames(operations []tracestore.Operation) []string {
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

func convertTagsToAttributes(tags map[string]string) pcommon.Map {
	attrs := pcommon.NewMap()
	for k, v := range tags {
		attrs.PutStr(k, v)
	}
	return attrs
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
