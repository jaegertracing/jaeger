// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/jaegertracing/jaeger/internal/storage/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
)

//go:embed v004-go-tmpl.cql.tmpl
var schemaFile embed.FS

type templateParams struct {
	// Keyspace in which tables and types will be created for storage
	Keyspace string
	// Replication is the replication strategy used. Ex: "{'class': 'NetworkTopologyStrategy', 'replication_factor': '1' }"
	Replication string
	// CompactionWindowInMinutes is constructed from CompactionWindow for using in template
	CompactionWindowInMinutes int64
	// TraceTTLInSeconds is constructed from TraceTTL for using in template
	TraceTTLInSeconds int64
	// DependenciesTTLInSeconds is constructed from DependenciesTTL for using in template
	DependenciesTTLInSeconds int64
}

type Creator struct {
	session cassandra.Session
	schema  config.Schema
}

// NewSchemaCreator returns a new SchemaCreator
func NewSchemaCreator(session cassandra.Session, schema config.Schema) *Creator {
	return &Creator{
		session: session,
		schema:  schema,
	}
}

func (sc *Creator) constructTemplateParams() templateParams {
	return templateParams{
		Keyspace:                  sc.schema.Keyspace,
		Replication:               fmt.Sprintf("{'class': 'NetworkTopologyStrategy', 'replication_factor': '%v' }", sc.schema.ReplicationFactor),
		CompactionWindowInMinutes: int64(sc.schema.CompactionWindow / time.Minute),
		TraceTTLInSeconds:         int64(sc.schema.TraceTTL / time.Second),
		DependenciesTTLInSeconds:  int64(sc.schema.DependenciesTTL / time.Second),
	}
}

func (*Creator) getQueryFileAsBytes(fileName string, params templateParams) ([]byte, error) {
	tmpl, err := template.ParseFS(schemaFile, fileName)
	if err != nil {
		return nil, err
	}

	var result bytes.Buffer
	err = tmpl.Execute(&result, params)
	if err != nil {
		return nil, err
	}

	return result.Bytes(), nil
}

func (*Creator) getQueriesFromBytes(queryFile []byte) ([]string, error) {
	lines := bytes.Split(queryFile, []byte("\n"))

	var extractedLines [][]byte

	for _, line := range lines {
		// Remove any comments, if at the end of the line
		commentIndex := bytes.Index(line, []byte(`--`))
		if commentIndex != -1 {
			// remove everything after comment
			line = line[0:commentIndex]
		}

		trimmedLine := bytes.TrimSpace(line)

		if len(trimmedLine) == 0 {
			continue
		}

		extractedLines = append(extractedLines, trimmedLine)
	}

	var queries []string

	// Construct individual queries strings
	var queryString string
	for _, line := range extractedLines {
		queryString += string(line) + "\n"
		if bytes.HasSuffix(line, []byte(";")) {
			queries = append(queries, queryString)
			queryString = ""
		}
	}

	if len(queryString) > 0 {
		return nil, errors.New(`query exists in template without ";"`)
	}

	return queries, nil
}

func (sc *Creator) getCassandraQueriesFromQueryStrings(queries []string) []cassandra.Query {
	var casQueries []cassandra.Query

	for _, query := range queries {
		casQueries = append(casQueries, sc.session.Query(query))
	}

	return casQueries
}

func (sc *Creator) contructSchemaQueries() ([]cassandra.Query, error) {
	params := sc.constructTemplateParams()

	queryFile, err := sc.getQueryFileAsBytes(`v004-go-tmpl.cql.tmpl`, params)
	if err != nil {
		return nil, err
	}

	queryStrings, err := sc.getQueriesFromBytes(queryFile)
	if err != nil {
		return nil, err
	}

	casQueries := sc.getCassandraQueriesFromQueryStrings(queryStrings)

	return casQueries, nil
}

func (sc *Creator) CreateSchemaIfNotPresent() error {
	casQueries, err := sc.contructSchemaQueries()
	if err != nil {
		return err
	}

	for _, query := range casQueries {
		if err := query.Exec(); err != nil {
			return err
		}
	}

	return nil
}
