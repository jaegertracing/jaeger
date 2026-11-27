// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"time"
)

func CalculateDeletionCutoff(currTime time.Time, numOfDays int) time.Time {
	return currTime.Add(-time.Hour * 24 * time.Duration(numOfDays))
}
