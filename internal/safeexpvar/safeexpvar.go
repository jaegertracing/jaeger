// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package safeexpvar

import (
	"expvar"
	"sync"
)

// mu serializes the get-or-create below: expvar.NewInt panics if two
// goroutines both observe a missing name and try to publish it.
var mu sync.Mutex

func SetInt(name string, value int64) {
	mu.Lock()
	v := expvar.Get(name)
	if v == nil {
		v = expvar.NewInt(name)
	}
	mu.Unlock()
	v.(*expvar.Int).Set(value)
}
