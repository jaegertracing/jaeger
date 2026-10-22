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

// NonFiniteFloatToken parses the OTLP/JSON tokens "NaN", "Infinity", and
// "-Infinity". Other strings, including "Inf" and "-Inf", are not tokens.
func NonFiniteFloatToken(s string) (float64, bool) {
	switch s {
	case "NaN":
		return math.NaN(), true
	case "Infinity":
		return math.Inf(1), true
	case "-Infinity":
		return math.Inf(-1), true
	default:
		return 0, false
	}
}
