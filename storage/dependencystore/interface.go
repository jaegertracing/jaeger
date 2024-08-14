// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
