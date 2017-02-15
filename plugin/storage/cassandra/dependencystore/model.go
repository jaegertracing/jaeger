// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package dependencystore

import (
	"fmt"

	"github.com/gocql/gocql"
)

// Dependency is the UDT representation of a Jaeger Dependency.
type Dependency struct {
	Parent    string `cql:"parent"`
	Child     string `cql:"child"`
	CallCount int64  `cql:"call_count"` // aways unsigned, but we cannot explicitly read uint64 from Cassandra
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
