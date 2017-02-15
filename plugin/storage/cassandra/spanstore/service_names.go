// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package spanstore

import (
	"time"

	"github.com/pkg/errors"
	"github.com/uber-go/zap"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger/pkg/cache"
	"github.com/uber/jaeger/pkg/cassandra"
	casMetrics "github.com/uber/jaeger/pkg/cassandra/metrics"
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
	logger        zap.Logger
}

// NewServiceNamesStorage returns a new ServiceNamesStorage
func NewServiceNamesStorage(
	session cassandra.Session,
	writeCacheTTL time.Duration,
	metricsFactory metrics.Factory,
	logger zap.Logger,
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
