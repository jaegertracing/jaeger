// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/pkg/cassandra"
	casMetrics "github.com/jaegertracing/jaeger/pkg/cassandra/metrics"
)

const (
	insertServiceName = `INSERT INTO service_names(service_name) VALUES (?)`
	queryServiceNames = `SELECT service_name FROM service_names`
)

// ServiceNamesStorage stores known service names.
type ServiceNamesStorage struct {
	session       cassandra.Session
	writeCacheTTL time.Duration
	InsertStmt    string
	QueryStmt     string
	metrics       *casMetrics.Table
	serviceNames  cache.Cache
	logger        *zap.Logger
}

// NewServiceNamesStorage returns a new ServiceNamesStorage
func NewServiceNamesStorage(
	session cassandra.Session,
	writeCacheTTL time.Duration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) *ServiceNamesStorage {
	return &ServiceNamesStorage{
		session:       session,
		InsertStmt:    insertServiceName,
		QueryStmt:     queryServiceNames,
		metrics:       casMetrics.NewTable(metricsFactory, "service_names"),
		writeCacheTTL: writeCacheTTL,
		logger:        logger,
		serviceNames: cache.NewLRUWithOptions(
			10000,
			&cache.Options{
				TTL:             writeCacheTTL,
				InitialCapacity: 1000,
			}),
	}
}

// Write saves a single service name
func (s *ServiceNamesStorage) Write(serviceName string) error {
	var err error
	query := s.session.Query(s.InsertStmt)
	if inCache := checkWriteCache(serviceName, s.serviceNames, s.writeCacheTTL); !inCache {
		q := query.Bind(serviceName)
		err2 := s.metrics.Exec(q, s.logger)
		if err2 != nil {
			err = err2
		}
	}
	return err
}

// checks if the key is in cache; returns true if it is, otherwise puts it there and returns false
func checkWriteCache(key string, c cache.Cache, writeCacheTTL time.Duration) bool {
	if writeCacheTTL == 0 {
		return false
	}
	// even though there is a race condition between Get and Put, it's not a problem for storage,
	// it simply means we might write the same service name twice.
	inCache := c.Get(key)
	if inCache == nil {
		c.Put(key, key)
	}
	return inCache != nil
}

// GetServices returns all services traced by Jaeger
func (s *ServiceNamesStorage) GetServices() ([]string, error) {
	iter := s.session.Query(s.QueryStmt).Iter()

	var service string
	var services []string
	for iter.Scan(&service) {
		services = append(services, service)
	}
	if err := iter.Close(); err != nil {
		err = fmt.Errorf("error reading service_names from storage: %w", err)
		return nil, err
	}
	return services, nil
}
