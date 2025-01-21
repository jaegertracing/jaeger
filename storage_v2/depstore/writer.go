// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"time"

	"github.com/jaegertracing/jaeger/model"
)

// Writer write the dependencies into the storage
type Writer interface {
	WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error
}
