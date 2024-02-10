// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"time"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
)

type TimeThroughput struct {
	Timestamp  time.Time           `json:"timestamp"`
	Throughput []*model.Throughput `json:"throughputs"`
}

type TimeProbabilitiesAndQPS struct {
	Timestamp           time.Time           `json:"timestamp"`
	ProbabilitiesAndQPS ProbabilitiesAndQPS `json:"probabilitiesandqps"`
}

type ProbabilitiesAndQPS struct {
	Hostname      string
	Probabilities model.ServiceOperationProbabilities
	QPS           model.ServiceOperationQPS
}
