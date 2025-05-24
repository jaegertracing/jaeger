// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// Dependency represents a row in the ClickHouse 'dependencies' table.
type Dependency struct {
	Parent    string    `ch:"parent"`
	Child     string    `ch:"child"`
	CallCount uint64    `ch:"call_count"`
	Source    string    `ch:"source"`
	Timestamp time.Time `ch:"timestamp"`
}

// toModel maps a Dependency (DB representation) to the model.DependencyLink used in the API layer.
func (d *Dependency) toModel() model.DependencyLink {
	return model.DependencyLink{
		Parent:    d.Parent,
		Child:     d.Child,
		CallCount: d.CallCount,
		Source:    d.Source,
	}.ApplyDefaults()
}
