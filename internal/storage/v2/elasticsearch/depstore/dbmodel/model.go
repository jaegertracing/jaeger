// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import "time"

// TimeDependencies encapsulates dependencies created at a given time
type TimeDependencies struct {
	Timestamp    time.Time        `json:"timestamp"`
	Dependencies []DependencyLink `json:"dependencies"`
}

// DependencyLink shows dependencies between services
type DependencyLink struct {
	Parent    string `json:"parent"`
	Child     string `json:"child"`
	CallCount uint64 `json:"callCount"`
}
