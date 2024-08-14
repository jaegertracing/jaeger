// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package samplingstore

import (
	"time"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
)

// Store writes and retrieves sampling data to and from storage.
type Store interface {
	// InsertThroughput inserts aggregated throughput for operations into storage.
	InsertThroughput(throughput []*model.Throughput) error

	// InsertProbabilitiesAndQPS inserts calculated sampling probabilities and measured qps into storage.
	InsertProbabilitiesAndQPS(hostname string, probabilities model.ServiceOperationProbabilities, qps model.ServiceOperationQPS) error

	// GetThroughput retrieves aggregated throughput for operations within a time range.
	GetThroughput(start, end time.Time) ([]*model.Throughput, error)

	// GetLatestProbabilities retrieves the latest sampling probabilities.
	GetLatestProbabilities() (model.ServiceOperationProbabilities, error)
}
