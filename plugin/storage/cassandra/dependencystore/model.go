// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dependencystore

import (
	"fmt"

	"github.com/gocql/gocql"
)

// Dependency is the UDT representation of a Jaeger Dependency.
type Dependency struct {
	Parent    string `cql:"parent"`
	Child     string `cql:"child"`
	CallCount int64  `cql:"call_count"` // always unsigned, but we cannot explicitly read uint64 from Cassandra
	Source    string `cql:"source"`
}

// MarshalUDT handles marshalling a Dependency.
func (d *Dependency) MarshalUDT(name string, info gocql.TypeInfo) ([]byte, error) {
	switch name {
	case "parent":
		return gocql.Marshal(info, d.Parent)
	case "child":
		return gocql.Marshal(info, d.Child)
	case "call_count":
		return gocql.Marshal(info, d.CallCount)
	case "source":
		return gocql.Marshal(info, d.Source)
	default:
		return nil, fmt.Errorf("unknown column for position: %q", name)
	}
}

// UnmarshalUDT handles unmarshalling a Dependency.
func (d *Dependency) UnmarshalUDT(name string, info gocql.TypeInfo, data []byte) error {
	switch name {
	case "parent":
		return gocql.Unmarshal(info, data, &d.Parent)
	case "child":
		return gocql.Unmarshal(info, data, &d.Child)
	case "call_count":
		return gocql.Unmarshal(info, data, &d.CallCount)
	case "source":
		return gocql.Unmarshal(info, data, &d.Source)
	default:
		return fmt.Errorf("unknown column for position: %q", name)
	}
}
