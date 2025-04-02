// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

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
	Query(stmt string, values ...any) Query
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
	ScanCAS(dest ...any) (bool, error)
}

// Query is an abstraction of gocql.Query
type Query interface {
	UpdateQuery
	Iter() Iterator
	Bind(v ...any) Query
	Consistency(level Consistency) Query
	PageSize(int) Query
}

// Iterator is an abstraction of gocql.Iter
type Iterator interface {
	Scan(dest ...any) bool
	Close() error
}
