// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package delay

import (
	"math"
	"math/rand"
	"time"
)

// Sleep generates a normally distributed random delay with given mean and stdDev
// and blocks for that duration.
func Sleep(mean time.Duration, stdDev time.Duration) {
	fMean := float64(mean)
	fStdDev := float64(stdDev)
	delay := time.Duration(math.Max(1, rand.NormFloat64()*fStdDev+fMean))
	time.Sleep(delay)
}
