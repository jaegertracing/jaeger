package cassandra

import (
	"github.com/pkg/errors"

	"github.com/uber/jaeger/pkg/cassandra"
)

// CQLLock is a distributed lock based off Cassandra.
type CQLLock struct {
	session  cassandra.Session
	tenantID string
}

const (
	leasesTable   = `leases`
	cqlInsertLock = `INSERT INTO ` + leasesTable + ` (name, owner) VALUES (?,?) IF NOT EXISTS;`
	cqlUpdateLock = `UPDATE ` + leasesTable + ` set owner = ? where name = ? IF owner = ?;`
)

var (
	errLockOwnership = errors.New("This host does not own the resource lock")
)

// NewCQLLock creates a new instance of a distributed locking mechanism based off Cassandra.
func NewCQLLock(session cassandra.Session, tenantID string) *CQLLock {
	return &CQLLock{
		session:  session,
		tenantID: tenantID,
	}
}

// Acquire acquires a lock around a given resource.
func (l *CQLLock) Acquire(resource string) (bool, error) {
	var name, owner string
	applied, err := l.session.Query(cqlInsertLock, resource, l.tenantID).ScanCAS(&name, &owner)
	if err != nil {
		return false, errors.Wrap(err, "Failed to acquire resource lock due to cassandra error")
	}
	if applied {
		// The lock was successfully created
		return true, nil
	}
	if owner == l.tenantID {
		// This host already owns the lock, extend the lease
		if err = l.extendLease(resource); err != nil {
			return false, errors.Wrap(err, "Failed to extend lease on resource lock")
		}
		return true, nil
	}
	return false, nil
}

// extendLease will attempt to extend the lease of an existing lock on a given resource.
func (l *CQLLock) extendLease(resource string) error {
	var owner string
	applied, err := l.session.Query(cqlUpdateLock, l.tenantID, resource, l.tenantID).ScanCAS(&owner)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	return errLockOwnership
}
