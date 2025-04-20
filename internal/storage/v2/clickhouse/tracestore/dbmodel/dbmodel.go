// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

// Trace Domain model in Clickhouse.
// This struct represents the schema for storing OTel pipeline Traces in Clickhouse.
type Trace struct {
	Resource Resource
	Scope    Scope
	Span     Span
	Links    []Link
	Events   []Event
}

type Resource struct {
	Attributes AttributesGroup
}

type Scope struct {
	Attributes AttributesGroup
}

type Span struct {
	Attributes AttributesGroup
}

type Link struct {
	Attributes AttributesGroup
}

type Event struct {
	Attributes AttributesGroup
}
