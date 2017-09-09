// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	default:
		return fmt.Errorf("unknown column for position: %q", name)
	}
}
