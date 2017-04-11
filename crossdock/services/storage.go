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
	"time"

	"github.com/gocql/gocql"
	"go.uber.org/zap"

	"github.com/uber/jaeger/pkg/cassandra"
	gocqlw "github.com/uber/jaeger/pkg/cassandra/gocql"
)

const (
	scriptsDir = "/go/scripts/"
	schema     = scriptsDir + "schema.cql"

	dropKeyspaceQuery = "DROP KEYSPACE IF EXISTS %s"

	jaegerKeyspace = "jaeger"

	cassandraHost = "cassandra"
)

// InitializeStorage initializes cassandra instances.
func InitializeStorage(logger *zap.Logger) {
	session := initializeCassandra(logger, cassandraHost, 4)
	if session == nil {
		logger.Fatal("Failed to initialize cassandra session")
	}
	logger.Info("Initialized cassandra session")
	initializeCassandraSchema(logger, schema, jaegerKeyspace, session)
}

func newCassandraCluster(host string, protoVersion int) (cassandra.Session, error) {
	cluster := gocql.NewCluster(host)
	cluster.ProtoVersion = protoVersion
	cluster.Timeout = 30 * time.Second
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, err
	}
	return gocqlw.WrapCQLSession(session), nil
}

func initializeCassandra(logger *zap.Logger, host string, protoVersion int) cassandra.Session {
	var session cassandra.Session
	var err error
	for i := 0; i < 30; i++ {
		session, err = newCassandraCluster(host, protoVersion)
		if err == nil {
			break
		}
		logger.Warn("Failed to initialize cassandra session", zap.String("host", host), zap.Error(err))
		time.Sleep(1 * time.Second)
	}
	return session
}

func initializeCassandraSchema(logger *zap.Logger, schemaFile, keyspace string, session cassandra.Session) {
	buf, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		logger.Fatal("Could not load schema file", zap.String("file", schemaFile), zap.Error(err))
	}
	file := string(buf)
	// (NB). Queries are split by 2 newlines, not an exact science
	schemaQueries := strings.Split(file, "\n\n")
	schemaQueries = append([]string{fmt.Sprintf(dropKeyspaceQuery, keyspace)}, schemaQueries...)
	for _, schemaQuery := range schemaQueries {
		if err := session.Query(schemaQuery).Exec(); err != nil {
			logger.Error("Failed to apply a schema query", zap.String("query", schemaQuery), zap.Error(err))
		}
	}
}
