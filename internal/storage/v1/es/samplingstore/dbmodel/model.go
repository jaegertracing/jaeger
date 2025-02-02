// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"time"
)

type Throughput struct {
	Service       string
	Operation     string
	Count         int64
	Probabilities map[string]struct{}
}

type TimeThroughput struct {
	Timestamp  time.Time  `json:"timestamp"`
	Throughput Throughput `json:"throughputs"`
}

type ProbabilitiesAndQPS struct {
	Hostname      string
	Probabilities map[string]map[string]float64
	QPS           map[string]map[string]float64
}

type TimeProbabilitiesAndQPS struct {
	Timestamp           time.Time           `json:"timestamp"`
	ProbabilitiesAndQPS ProbabilitiesAndQPS `json:"probabilitiesandqps"`
}
