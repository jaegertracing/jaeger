// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
)

var _ depstore.Reader = (*Reader)(nil)

type Reader struct{}

func NewDependencyReader() *Reader {
	return &Reader{}
}

func (*Reader) GetDependencies(ctx context.Context, query depstore.QueryParameters) ([]model.DependencyLink, error) {
	return nil, nil
}
