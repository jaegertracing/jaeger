// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package distributedlock

import (
	"time"
)

// Lock uses distributed lock for control of a resource.
type Lock interface {
	// Acquire acquires a lease of duration ttl around a given resource. In case of an error,
	// acquired is meaningless.
	Acquire(resource string, ttl time.Duration) (acquired bool, err error)

	// Forfeit forfeits a lease around a given resource. In case of an error,
	// forfeited is meaningless.
	Forfeit(resource string) (forfeited bool, err error)
}
