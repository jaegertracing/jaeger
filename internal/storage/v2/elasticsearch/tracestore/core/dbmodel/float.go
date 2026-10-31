// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import "math"

// JSONSafeFloat returns f when it is finite. NaN, +Inf, and -Inf are returned
// as the strings "NaN", "Infinity", and "-Infinity", the tokens OTLP JSON uses,
// because encoding/json rejects the bare floats.
func JSONSafeFloat(f float64) any {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	default:
		return f
	}
}
