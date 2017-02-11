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
			typeInfo := *(*gocql.NativeType)(unsafe.Pointer(&nt))
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
