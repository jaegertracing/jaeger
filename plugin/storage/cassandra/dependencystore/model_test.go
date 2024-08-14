// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

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
