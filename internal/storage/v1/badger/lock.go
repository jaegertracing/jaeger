// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import "time"

type lock struct{}

// Acquire always returns true for badgerdb as no lock is needed
func (*lock) Acquire(string /* resource */, time.Duration /* ttl */) (bool, error) {
	return true, nil
}

// Forfeit always returns true for badgerdb as no lock is needed
func (*lock) Forfeit(string /* resource */) (bool, error) {
	return true, nil
}
