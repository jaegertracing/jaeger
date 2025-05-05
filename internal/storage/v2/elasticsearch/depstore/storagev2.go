// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore/dbmodel"
)

type DependencyStoreV2 struct {
	store CoreDependencyStore
}

// NewDependencyStoreV2 returns a DependencyStoreV2
func NewDependencyStoreV2(p Params) *DependencyStoreV2 {
	return &DependencyStoreV2{
		store: NewDependencyStore(p),
	}
}

func (s *DependencyStoreV2) GetDependencies(ctx context.Context, query depstore.QueryParameters) ([]model.DependencyLink, error) {
	dbDependencies, err := s.store.GetDependencies(ctx, query.EndTime, query.EndTime.Sub(query.StartTime))
	if err != nil {
		return nil, err
	}
	dependencies := dbmodel.ToDomainDependencies(dbDependencies)
	return dependencies, nil
}

func (s *DependencyStoreV2) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	dbDependencies := dbmodel.FromDomainDependencies(dependencies)
	return s.store.WriteDependencies(ts, dbDependencies)
}
