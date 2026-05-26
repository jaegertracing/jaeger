// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// Writer write the dependencies into the storage
type Writer interface {
	WriteDependencies(ctx context.Context, ts time.Time, dependencies []model.DependencyLink) error
}
