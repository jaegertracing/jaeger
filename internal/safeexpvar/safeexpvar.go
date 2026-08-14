// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package safeexpvar

import (
	"expvar"
	"sync"
)

// createMu serializes the get-or-create step. expvar offers no atomic
// equivalent: Get and NewInt are individually safe, but between them two
// goroutines can both observe nil for the same name and both call NewInt, and
// the second panics with "Reuse of exported var name". Publishing a var is rare
// and happens on startup paths, so a package-level mutex costs nothing.
var createMu sync.Mutex

// SetInt publishes value under name, creating the expvar.Int on first use.
// It is safe for concurrent use, including concurrent first use of the same name.
func SetInt(name string, value int64) {
	v := expvar.Get(name)
	if v == nil {
		createMu.Lock()
		// Re-check under the lock: another goroutine may have published name
		// while this one was blocked, and NewInt would panic on the duplicate.
		if v = expvar.Get(name); v == nil {
			v = expvar.NewInt(name)
		}
		createMu.Unlock()
	}
	v.(*expvar.Int).Set(value)
}
