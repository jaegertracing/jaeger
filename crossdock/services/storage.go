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

package services

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/uber/jaeger/pkg/cassandra"
)

// InitializeCassandraSchema applies a schema to the cassandra keyspace
func InitializeCassandraSchema(session cassandra.Session, schemaFile, keyspace string, logger *zap.Logger) error {
	logger.Info("Creating Cassandra schema", zap.String("keyspace", keyspace), zap.String("file", schemaFile))
	buf, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		return err
	}
	file := string(buf)
	// Split file into individual queries.
	schemaQueries := strings.Split(file, ";")
	dropKeyspaceQuery := fmt.Sprintf("DROP KEYSPACE IF EXISTS %s", keyspace)
	schemaQueries = append([]string{dropKeyspaceQuery}, schemaQueries...)
	for _, schemaQuery := range schemaQueries {
		schemaQuery = strings.TrimSpace(schemaQuery)
		if schemaQuery == "" {
			continue
		}
		logger.Info("Executing", zap.String("query", schemaQuery))
		if err := session.Query(schemaQuery + ";").Exec(); err != nil {
			return errors.Wrap(err, fmt.Sprintf("Failed to apply a schema query: %s", schemaQuery))
		}
	}
	return nil
}
