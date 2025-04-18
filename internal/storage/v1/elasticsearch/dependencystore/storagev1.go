// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencystore

import (
	"context"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/dependencystore/dbmodel"
)

var (
	_ dependencystore.Reader = &StoreV1{} // check API conformance
	_ dependencystore.Writer = &StoreV1{} // check API conformance
)

type StoreV1 struct {
	depStore CoreDependencyStore
}

// NewDependencyStoreV1 returns a StoreV1
func NewDependencyStoreV1(p Params) *StoreV1 {
	return &StoreV1{
		depStore: NewDependencyStore(p),
	}
}

// WriteDependencies implements dependencystore.Writer#WriteDependencies.
func (s *StoreV1) WriteDependencies(ts time.Time, dependencies []model.DependencyLink) error {
	dbDependencies := dbmodel.FromDomainDependencies(dependencies)
	return s.depStore.WriteDependencies(ts, dbDependencies)
}

// CreateTemplates creates index templates.
func (s *StoreV1) CreateTemplates(dependenciesTemplate string) error {
	return s.depStore.CreateTemplates(dependenciesTemplate)
}

// GetDependencies returns all interservice dependencies
func (s *StoreV1) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	dbDependencies, err := s.depStore.GetDependencies(ctx, endTs, lookback)
	if err != nil {
		return nil, err
	}
	return dbmodel.ToDomainDependencies(dbDependencies), nil
}
