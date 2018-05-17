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

package testutils

import (
	"testing"
	"unsafe"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
)

// UDTField describes a field in a gocql User Defined Type
type UDTField struct {
	Name  string
	Type  gocql.Type
	ValIn []byte // value to attempt to marshal
	Err   bool   // is error expected?
}

// UDTTestCase desribes a test for a UDT
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
				proto  byte
				typ    gocql.Type
				custom string
			}{
				proto: 0x03,
				typ:   field.Type,
			}
			typeInfo := *(*gocql.NativeType)(unsafe.Pointer(&nt)) /* nolint #nosec */
			data, err := testCase.Obj.MarshalUDT(field.Name, typeInfo)
			if field.Err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, field.ValIn, data)
			}
			obj := testCase.New()
			err = obj.UnmarshalUDT(field.Name, typeInfo, field.ValIn)
			if field.Err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
