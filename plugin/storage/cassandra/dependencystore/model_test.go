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
	"testing"

	"github.com/gocql/gocql"

	"github.com/uber/jaeger/pkg/cassandra/gocql/testutils"
)

func TestDependencyUDT(t *testing.T) {
	dependency := &Dependency{
		Parent:    "goo",
		Child:     "gle",
		CallCount: 123,
	}

	testCase := testutils.UDTTestCase{
		Obj:     dependency,
		New:     func() gocql.UDTUnmarshaler { return &Dependency{} },
		ObjName: "Dependency",
		Fields: []testutils.UDTField{
			{Name: "parent", Type: gocql.TypeAscii, ValIn: []byte("goo"), Err: false},
			{Name: "child", Type: gocql.TypeAscii, ValIn: []byte("gle"), Err: false},
			{Name: "call_count", Type: gocql.TypeBigInt, ValIn: []byte{0, 0, 0, 0, 0, 0, 0, 123}, Err: false},
			{Name: "wrong-field", Err: true},
		},
	}
	testCase.Run(t)
}
