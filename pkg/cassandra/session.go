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

package cassandra

import (
	"github.com/gocql/gocql"
)

// Consistency is Cassandra's consistency level for queries.
type Consistency uint16

const (
	// Any ...
	Any Consistency = 0x00
	// One ...
	One Consistency = 0x01
	// Two ...
	Two Consistency = 0x02
	// Three ...
	Three Consistency = 0x03
	// Quorum ...
	Quorum Consistency = 0x04
	// All ...
	All Consistency = 0x05
	// LocalQuorum ...
	LocalQuorum Consistency = 0x06
	// EachQuorum ...
	EachQuorum Consistency = 0x07
	// LocalOne ...
	LocalOne Consistency = 0x0A
)

// Session is an abstraction of gocql.Session
type Session interface {
	Query(stmt string, values ...interface{}) Query
	Close()
}

// UpdateQuery is a subset of Query just for updates
type UpdateQuery interface {
	Exec() error
	String() string
}

// Query is an abstraction of gocql.Query
type Query interface {
	UpdateQuery
	Iter() Iterator
	Bind(v ...interface{}) Query
	Consistency(level Consistency) Query
}

// Iterator is an abstraction of gocql.Iter
type Iterator interface {
	Scan(dest ...interface{}) bool
	Close() error
	Columns() []gocql.ColumnInfo
}
