// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger/model"
)

// QueryParameters contains the parameters that can be used to query dependencies.
type QueryParameters struct {
	StartTime time.Time
	EndTime   time.Time
}

// Reader can load service dependencies from storage.
type Reader interface {
	GetDependencies(ctx context.Context, query QueryParameters) ([]model.DependencyLink, error)
}
