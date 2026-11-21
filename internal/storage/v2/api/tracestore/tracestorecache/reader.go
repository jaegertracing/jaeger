// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestorecache

import (
	"context"
	"slices"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

var _ tracestore.Reader = (*Reader)(nil)

type Reader struct {
	tracestore.Reader
	ttl        time.Duration
	logger     *zap.Logger
	timeNow    func() time.Time
	services   cache[string, string]
	operations cache[operationsKey, tracestore.Operation]
}

type operationsKey struct {
	tenant string
	query  tracestore.OperationQueryParams
}

type cache[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]*entry[V]
}

type entry[V any] struct {
	values     []V
	loadedAt   time.Time
	refreshing bool
}

func NewReader(reader tracestore.Reader, ttl time.Duration, logger *zap.Logger) *Reader {
	return &Reader{
		Reader:  reader,
		ttl:     ttl,
		logger:  logger,
		timeNow: time.Now,
	}
}

func (r *Reader) GetServices(ctx context.Context) ([]string, error) {
	return get(ctx, r, &r.services, tenancy.GetTenant(ctx), r.Reader.GetServices)
}

func (r *Reader) GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	key := operationsKey{tenant: tenancy.GetTenant(ctx), query: query}
	return get(ctx, r, &r.operations, key, func(ctx context.Context) ([]tracestore.Operation, error) {
		return r.Reader.GetOperations(ctx, query)
	})
}

func get[K comparable, V any](
	ctx context.Context,
	r *Reader,
	c *cache[K, V],
	key K,
	load func(context.Context) ([]V, error),
) ([]V, error) {
	c.mu.Lock()
	cached, found := c.entries[key]
	if found && (cached.refreshing || r.timeNow().Sub(cached.loadedAt) < r.ttl) {
		values := slices.Clone(cached.values)
		c.mu.Unlock()
		return values, nil
	}
	if found {
		cached.refreshing = true
	}
	c.mu.Unlock()

	values, err := load(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		if !found {
			return nil, err
		}
		cached.refreshing = false
		r.logger.Warn("Cannot refresh cached service or operation names, serving the cached ones", zap.Error(err))
		return slices.Clone(cached.values), nil
	}
	if len(values) == 0 {
		delete(c.entries, key)
		return values, nil
	}
	if c.entries == nil {
		c.entries = make(map[K]*entry[V])
	}
	c.entries[key] = &entry[V]{values: slices.Clone(values), loadedAt: r.timeNow()}
	return values, nil
}
