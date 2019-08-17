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

package cassandra

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

	// ScanCAS executes a lightweight transaction (i.e. an UPDATE or INSERT
	// statement containing an IF clause). If the transaction fails because
	// the existing values did not match, the previous values will be stored
	// in dest.
	ScanCAS(dest ...interface{}) (bool, error)
}

// Query is an abstraction of gocql.Query
type Query interface {
	UpdateQuery
	Iter() Iterator
	Bind(v ...interface{}) Query
	Consistency(level Consistency) Query
	PageSize(int) Query
}

// Iterator is an abstraction of gocql.Iter
type Iterator interface {
	Scan(dest ...interface{}) bool
	Close() error
}
