// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import "time"

type Lock struct{}

// Acquire always returns true for memory storage because it's a single-node
func (*Lock) Acquire(string /* resource */, time.Duration /* ttl */) (bool, error) {
	return true, nil
}

// Forfeit always returns true for memory storage
func (*Lock) Forfeit(string /* resource */) (bool, error) {
	return true, nil
}
