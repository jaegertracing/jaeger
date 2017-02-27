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

package gocql

import (
	"github.com/gocql/gocql"

	"github.com/uber/jaeger/pkg/cassandra"
)

// CQLSession is a wrapper around gocql.Session.
type CQLSession struct {
	session *gocql.Session
}

// WrapCQLSession creates a Session out of *gocql.Session.
func WrapCQLSession(session *gocql.Session) CQLSession {
	return CQLSession{session: session}
}

// Query delegates to gocql.Session#Query and wraps the result as Query.
func (s CQLSession) Query(stmt string, values ...interface{}) cassandra.Query {
	return WrapCQLQuery(s.session.Query(stmt, values...))
}

// Close delegates to gocql.Session#Close.
func (s CQLSession) Close() {
	s.session.Close()
}

// ---

// CQLQuery is a wrapper around gocql.Query.
type CQLQuery struct {
	query *gocql.Query
}

// WrapCQLQuery creates a Query out of *gocql.Query.
func WrapCQLQuery(query *gocql.Query) CQLQuery {
	return CQLQuery{query: query}
}

// Exec delegates to gocql.Query#Exec.
func (q CQLQuery) Exec() error {
	return q.query.Exec()
}

// ScanCAS delegates to gocql.Query#ScanCAS.
func (q CQLQuery) ScanCAS(dest ...interface{}) (bool, error) {
	return q.query.ScanCAS(dest...)
}

// Iter delegates to gocql.Query#Iter and wraps the result as Iterator.
func (q CQLQuery) Iter() cassandra.Iterator {
	return WrapCQLIterator(q.query.Iter())
}

// Bind delegates to gocql.Query#Bind and wraps the result as Query.
func (q CQLQuery) Bind(v ...interface{}) cassandra.Query {
	return WrapCQLQuery(q.query.Bind(v...))
}

// Consistency delegates to gocql.Query#Consistency and wraps the result as Query.
func (q CQLQuery) Consistency(level cassandra.Consistency) cassandra.Query {
	return WrapCQLQuery(q.query.Consistency(gocql.Consistency(level)))
}

// String returns string representation of this query.
func (q CQLQuery) String() string {
	return q.query.String()
}

// ---

// CQLIterator is a wrapper around gocql.Iter.
type CQLIterator struct {
	iter *gocql.Iter
}

// WrapCQLIterator creates an Iterator out of *gocql.Iter.
func WrapCQLIterator(iter *gocql.Iter) CQLIterator {
	return CQLIterator{iter: iter}
}

// Scan delegates to gocql.Iter#Scan.
func (i CQLIterator) Scan(dest ...interface{}) bool {
	return i.iter.Scan(dest...)
}

// Close delegates to gocql.Iter#Close.
func (i CQLIterator) Close() error {
	return i.iter.Close()
}
