// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

type IndexAPI interface {
	GetJaegerIndices(ctx context.Context, prefix string) ([]Index, error)
	IndexExists(ctx context.Context, index string) (bool, error)
	AliasExists(ctx context.Context, alias string) (bool, error)
	DeleteIndices(ctx context.Context, indices []Index) error
	CreateIndex(ctx context.Context, index string) error
	CreateAlias(ctx context.Context, aliases []Alias) error
	DeleteAlias(ctx context.Context, aliases []Alias) error
	CreateTemplate(ctx context.Context, template, name string) error
	Rollover(ctx context.Context, rolloverTarget string, conditions map[string]any) error
}

type ClusterAPI interface {
	ResolveVersion(ctx context.Context, configured uint) (es.BackendVersion, error)
}

type IndexManagementLifecycleAPI interface {
	Exists(ctx context.Context, name string) (bool, error)
}
