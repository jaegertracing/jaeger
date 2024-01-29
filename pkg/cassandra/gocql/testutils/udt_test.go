// Copyright (c) 2019 The Jaeger Authors.
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

package testutils_test

import (
	"testing"

	"github.com/gocql/gocql"
	"github.com/jaegertracing/jaeger/pkg/cassandra/gocql/testutils"
)

// CustomUDT is a custom type that implements gocql.UDTMarshaler and gocql.UDTUnmarshaler interfaces.
type CustomUDT struct {
	Field1 int
	Field2 string
}

// MarshalUDT implements the gocql.UDTMarshaler interface.
func (c *CustomUDT) MarshalUDT(name string, info gocql.TypeInfo) ([]byte, error) {
	switch name {
	case "Field1":
		return gocql.Marshal(info, c.Field1)
	case "Field2":
		return gocql.Marshal(info, c.Field2)
	default:
		return nil, gocql.ErrNotFound
	}
}

// UnmarshalUDT implements the gocql.UDTUnmarshaler interface.
func (c *CustomUDT) UnmarshalUDT(name string, info gocql.TypeInfo, data []byte) error {
	switch name {
	case "Field1":
		return gocql.Unmarshal(info, data, &c.Field1)
	case "Field2":
		return gocql.Unmarshal(info, data, &c.Field2)
	default:
		return gocql.ErrNotFound
	}
}

func TestUDTTestCase(t *testing.T) {
	// Create an example UDT instance
	udtInstance := &CustomUDT{
		Field1: 42,
		Field2: "test",
	}

	// Define UDT fields for testing
	udtFields := []testutils.UDTField{
		{
			Name:  "Field1",
			Type:  gocql.TypeBigInt,
			ValIn: []byte{0, 0, 0, 0, 0, 0, 0, 42},
			Err:   false,
		},
		{
			Name:  "Field2",
			Type:  gocql.TypeVarchar,
			ValIn: []byte("test"),
			Err:   false,
		},
		// Add more UDTField entries as needed
	}

	// Create a UDTTestCase
	testCase := testutils.UDTTestCase{
		Obj:     udtInstance,
		ObjName: "CustomUDT",
		New:     func() gocql.UDTUnmarshaler { return &CustomUDT{} },
		Fields:  udtFields,
	}

	// Run the test case
	testCase.Run(t)
}
