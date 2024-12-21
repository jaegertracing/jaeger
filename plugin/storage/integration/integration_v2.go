// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
)

// StorageIntegrationV2 performs same functions as StorageIntegration
//
// However StorageIntegrationV2 upgrades storage integration tests
// to storage v2 tests
type StorageIntegrationV2 struct {
	TraceReader tracestore.Reader
}