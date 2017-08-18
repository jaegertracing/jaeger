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
	"time"

	"github.com/pkg/errors"

	"github.com/uber/jaeger/pkg/cassandra"
)

// Lock is a distributed lock based off Cassandra.
type Lock struct {
	session  cassandra.Session
	tenantID string
}

const (
	defaultTTL = 60 * time.Second

	leasesTable   = `leases`
	cqlInsertLock = `INSERT INTO ` + leasesTable + ` (name, owner) VALUES (?,?) IF NOT EXISTS USING TTL ?;`
	cqlUpdateLock = `UPDATE ` + leasesTable + ` USING TTL ? SET owner = ? WHERE name = ? IF owner = ?;`
	cqlDeleteLock = `DELETE FROM ` + leasesTable + ` WHERE name = ? IF owner = ?;`
)

var (
	errLockOwnership = errors.New("This host does not own the resource lock")
)

// NewLock creates a new instance of a distributed locking mechanism based off Cassandra.
func NewLock(session cassandra.Session, tenantID string) *Lock {
	return &Lock{
		session:  session,
		tenantID: tenantID,
	}
}

// Acquire acquires a lease around a given resource. NB. Cassandra only allows ttl of seconds granularity
func (l *Lock) Acquire(resource string, ttl time.Duration) (bool, error) {
	if ttl == 0 {
		ttl = defaultTTL
	}
	var name, owner string
	applied, err := l.session.Query(cqlInsertLock, resource, l.tenantID, ttl.Seconds()).ScanCAS(&name, &owner)
	if err != nil {
		return false, errors.Wrap(err, "Failed to acquire resource lock due to cassandra error")
	}
	if applied {
		// The lock was successfully created
		return true, nil
	}
	if owner == l.tenantID {
		// This host already owns the lock, extend the lease
		if err = l.extendLease(resource, ttl); err != nil {
			return false, errors.Wrap(err, "Failed to extend lease on resource lock")
		}
		return true, nil
	}
	return false, nil
}

func (l *Lock) Forfeit(resource string) (bool, error) {
	var name, owner string
	applied, err := l.session.Query(cqlDeleteLock, resource, l.tenantID).ScanCAS(&name, &owner)
	if err != nil {
		return false, errors.Wrap(err, "Failed to forfeit resource lock due to cassandra error")
	}
	if applied {
		// The lock was successfully deleted
		return true, nil
	}
	return false, errors.Wrap(errLockOwnership, "Failed to forfeit resource lock")
}

// extendLease will attempt to extend the lease of an existing lock on a given resource.
func (l *Lock) extendLease(resource string, ttl time.Duration) error {
	var owner string
	applied, err := l.session.Query(cqlUpdateLock, ttl.Seconds(), l.tenantID, resource, l.tenantID).ScanCAS(&owner)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	return errLockOwnership
}
