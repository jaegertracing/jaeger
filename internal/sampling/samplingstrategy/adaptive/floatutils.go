// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"math"
	"strconv"
)

// TruncateFloat truncates float to 6 decimal positions and converts to string.
func TruncateFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

// FloatEquals compares two floats with 10 decimal positions precision.
func FloatEquals(a, b float64) bool {
	return math.Abs(a-b) < 1e-10
}
