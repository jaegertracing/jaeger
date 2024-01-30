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
	udtInstance := &CustomUDT{
		Field1: 1,
		Field2: "test",
	}

	// Define UDT fields for testing
	udtFields := []testutils.UDTField{
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
			Name:  "InvalidType",
			Type:  gocql.TypeBigInt,
			ValIn: []byte("test"),
			Err:   true,
		},
		{
			Name:  "NilTypeInfo",
			Type:  gocql.TypeBigInt,
			ValIn: []byte{},
			Err:   true,
		},
		{
			Name:  "InvalidDataLength",
			Type:  gocql.TypeBigInt,
			ValIn: []byte{0, 0, 0, 0, 0, 0, 0},
			Err:   true,
		},
	}

	// Create a UDTTestCase
	testCase := testutils.UDTTestCase{
		Obj:     udtInstance,
		ObjName: "CustomUDT",
		New:     func() gocql.UDTUnmarshaler { return &CustomUDT{} },
		Fields:  udtFields,
	}

	testCase.Run(t)
}
