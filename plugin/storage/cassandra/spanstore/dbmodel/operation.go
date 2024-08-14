// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

// Operation defines schema for records saved in operation_names_v2 table
type Operation struct {
	ServiceName   string
	SpanKind      string
	OperationName string
}
