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

import (
	"errors"
	"fmt"
	"time"

	"github.com/jaegertracing/jaeger/pkg/cassandra"
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
	errLockOwnership = errors.New("this host does not own the resource lock")
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
	ttlSec := int(ttl.Seconds())
	var name, owner string
	applied, err := l.session.Query(cqlInsertLock, resource, l.tenantID, ttlSec).ScanCAS(&name, &owner)
	if err != nil {
		return false, fmt.Errorf("failed to acquire resource lock due to cassandra error: %w", err)
	}
	if applied {
		// The lock was successfully created
		return true, nil
	}
	if owner == l.tenantID {
		// This host already owns the lock, extend the lease
		if err = l.extendLease(resource, ttl); err != nil {
			return false, fmt.Errorf("failed to extend lease on resource lock: %w", err)
		}
		return true, nil
	}
	return false, nil
}

// Forfeit forfeits an existing lease around a given resource.
func (l *Lock) Forfeit(resource string) (bool, error) {
	var name, owner string
	applied, err := l.session.Query(cqlDeleteLock, resource, l.tenantID).ScanCAS(&name, &owner)
	if err != nil {
		return false, fmt.Errorf("failed to forfeit resource lock due to cassandra error: %w", err)
	}
	if applied {
		// The lock was successfully deleted
		return true, nil
	}
	return false, fmt.Errorf("failed to forfeit resource lock: %w", errLockOwnership)
}

// extendLease will attempt to extend the lease of an existing lock on a given resource.
func (l *Lock) extendLease(resource string, ttl time.Duration) error {
	ttlSec := int(ttl.Seconds())
	var owner string
	applied, err := l.session.Query(cqlUpdateLock, ttlSec, l.tenantID, resource, l.tenantID).ScanCAS(&owner)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	return errLockOwnership
}
