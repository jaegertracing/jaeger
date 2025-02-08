// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptive

import (
	"math"
	"strconv"
)

// truncateFloat truncates float to 6 decimal positions and converts to string.
func truncateFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

// floatEquals compares two floats with 10 decimal positions precision.
func floatEquals(a, b float64) bool {
	return math.Abs(a-b) < 1e-10
}
