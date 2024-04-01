// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package testutils_test

import (
	"testing"

	"github.com/gocql/gocql"

	gocqlutils "github.com/jaegertracing/jaeger/pkg/cassandra/gocql/testutils"
	"github.com/jaegertracing/jaeger/pkg/testutils"
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
	udtInstance := &CustomUDT{
		Field1: 1,
		Field2: "test",
	}

	// Define UDT fields for testing
	udtFields := []gocqlutils.UDTField{
		{
			Name:  "Field1",
			Type:  gocql.TypeBigInt,
			ValIn: []byte{0, 0, 0, 0, 0, 0, 0, 1},
			Err:   false,
		},
		{
			Name:  "Field2",
			Type:  gocql.TypeVarchar,
			ValIn: []byte("test"),
			Err:   false,
		},
		{
			Name:  "InvalidField",
			Type:  gocql.TypeBigInt,
			ValIn: []byte("test"),
			Err:   true,
		},
	}

	// Create a UDTTestCase
	testCase := gocqlutils.UDTTestCase{
		Obj:     udtInstance,
		ObjName: "CustomUDT",
		New:     func() gocql.UDTUnmarshaler { return &CustomUDT{} },
		Fields:  udtFields,
	}

	testCase.Run(t)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
