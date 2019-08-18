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

package dependencystore

import (
	"testing"

	"github.com/gocql/gocql"

	"github.com/jaegertracing/jaeger/pkg/cassandra/gocql/testutils"
)

func TestDependencyUDT(t *testing.T) {
	dependency := &Dependency{
		Parent:    "bi",
		Child:     "ng",
		CallCount: 123,
		Source:    "jaeger",
	}

	testCase := testutils.UDTTestCase{
		Obj:     dependency,
		New:     func() gocql.UDTUnmarshaler { return &Dependency{} },
		ObjName: "Dependency",
		Fields: []testutils.UDTField{
			{Name: "parent", Type: gocql.TypeAscii, ValIn: []byte("bi"), Err: false},
			{Name: "child", Type: gocql.TypeAscii, ValIn: []byte("ng"), Err: false},
			{Name: "call_count", Type: gocql.TypeBigInt, ValIn: []byte{0, 0, 0, 0, 0, 0, 0, 123}, Err: false},
			{Name: "source", Type: gocql.TypeAscii, ValIn: []byte("jaeger"), Err: false},
			{Name: "wrong-field", Err: true},
		},
	}
	testCase.Run(t)
}
