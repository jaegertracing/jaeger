// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package safeexpvar

import (
	"expvar"
)

func SetInt(name string, value int64) {
	v := expvar.Get(name)
	if v == nil {
		v = expvar.NewInt(name)
	}
	v.(*expvar.Int).Set(value)
}
