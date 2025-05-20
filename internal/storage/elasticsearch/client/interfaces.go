// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package client

import "context"

type IndexAPI interface {
	GetJaegerIndices(prefix string) ([]Index, error)
	IndexExists(index string) (bool, error)
	AliasExists(alias string) (bool, error)
	DeleteIndices(indices []Index) error
	CreateIndex(index string) error
	CreateAlias(aliases []Alias) error
	DeleteAlias(aliases []Alias) error
	CreateTemplate(template, name string) error
	Rollover(rolloverTarget string, conditions map[string]any) error
}

type ClusterAPI interface {
	Version() (uint, error)
}

type IndexManagementLifecycleAPI interface {
	Exists(ctx context.Context, name string) (bool, error)
}
