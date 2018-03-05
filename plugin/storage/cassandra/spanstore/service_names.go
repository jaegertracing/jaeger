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

package spanstore

import (
	"time"

	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/cache"
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
		metrics:       casMetrics.NewTable(metricsFactory, "ServiceNames"),
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
		err = errors.Wrap(err, "Error reading service_names from storage")
		return nil, err
	}
	return services, nil
}
