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

// Query delegates to gosql.Session#Query and wraps the result as Query.
func (s CQLSession) Query(stmt string, values ...interface{}) cassandra.Query {
	return WrapCQLQuery(s.session.Query(stmt, values...))
}

// Close delegates to gosql.Session#Close.
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

// Exec delegates to gosql.Query#Exec.
func (q CQLQuery) Exec() error {
	return q.query.Exec()
}

// Iter delegates to gosql.Query#Iter and wraps the result as Iterator.
func (q CQLQuery) Iter() cassandra.Iterator {
	return WrapCQLIterator(q.query.Iter())
}

// Bind delegates to gosql.Query#Bind and wraps the result as Query.
func (q CQLQuery) Bind(v ...interface{}) cassandra.Query {
	return WrapCQLQuery(q.query.Bind(v...))
}

// Consistency delegates to gosql.Query#Consistency and wraps the result as Query.
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

// Scan delegates to gosql.Iter#Scan.
func (i CQLIterator) Scan(dest ...interface{}) bool {
	return i.iter.Scan(dest...)
}

// Close delegates to gosql.Iter#Close.
func (i CQLIterator) Close() error {
	return i.iter.Close()
}
