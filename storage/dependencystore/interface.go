// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dependencystore

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/model"
)

// Writer stores service dependencies into storage.
type Writer interface {
	WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error
}

// Reader can load service dependencies from storage.
type Reader interface {
	GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error)
}
