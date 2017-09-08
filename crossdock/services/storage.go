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
