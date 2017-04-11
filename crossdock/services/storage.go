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
	scriptsDir     = "/go/scripts/"
	schema       = scriptsDir + "schema.cql"

	dropKeyspaceQuery = "DROP KEYSPACE IF EXISTS %s"

	jaegerKeyspace       = "jaeger"

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
