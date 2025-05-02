// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"testing"
	"unsafe"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UDTField describes a field in a gocql User Defined Type
type UDTField struct {
	Name  string
	Type  gocql.Type
	ValIn []byte // value to attempt to marshal
	Err   bool   // is error expected?
}

// UDTTestCase describes a test for a UDT
type UDTTestCase struct {
	Obj     gocql.UDTMarshaler
	ObjName string
	New     func() gocql.UDTUnmarshaler
	Fields  []UDTField
}

// Run runs a test case
func (testCase UDTTestCase) Run(t *testing.T) {
	for _, ff := range testCase.Fields {
		field := ff // capture loop var
		t.Run(testCase.ObjName+"-"+field.Name, func(t *testing.T) {
			// To test MarshalUDT we need a gocql.NativeType struct whose fields private.
			// Instead we create a structural copy that we cast to gocql.NativeType using unsafe.Pointer
			nt := struct {
				proto byte
				typ   gocql.Type
				_     string
			}{
				proto: 0x03,
				typ:   field.Type,
			}
			typeInfo := *(*gocql.NativeType)(unsafe.Pointer(&nt)) /* nolint #nosec */
			data, err := testCase.Obj.MarshalUDT(field.Name, typeInfo)
			if field.Err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, field.ValIn, data)
			}
			obj := testCase.New()
			err = obj.UnmarshalUDT(field.Name, typeInfo, field.ValIn)
			if field.Err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
