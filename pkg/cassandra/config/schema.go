// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/jaegertracing/jaeger/pkg/cassandra"
)

//go:embed v004-go-tmpl.cql.tmpl
var schemaFile embed.FS

type TemplateParams struct {
	Schema
	// Replication is the replication strategy used. Ex: "{'class': 'NetworkTopologyStrategy', 'replication_factor': '1' }"
	Replication string
	// CompactionWindowInMinutes is constructed from CompactionWindow for using in template
	CompactionWindowInMinutes int64
	// TraceTTLInSeconds is constructed from TraceTTL for using in template
	TraceTTLInSeconds int64
	// DependenciesTTLInSeconds is constructed from DependenciesTTL for using in template
	DependenciesTTLInSeconds int64
}

func constructTemplateParams(cfg Schema) (TemplateParams, error) {
	params := TemplateParams{
		Schema: cfg,
	}

	params.Replication = fmt.Sprintf("{'class': 'NetworkTopologyStrategy', 'replication_factor': '%v' }", params.ReplicationFactor)
	params.CompactionWindowInMinutes = int64(params.CompactionWindow / time.Minute)
	params.TraceTTLInSeconds = int64(params.TraceTTL / time.Second)
	params.DependenciesTTLInSeconds = int64(params.DependenciesTTL / time.Second)

	return params, nil
}

func getQueryFileAsBytes(fileName string, params TemplateParams) ([]byte, error) {
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

func getQueriesFromBytes(queryFile []byte) ([]string, error) {
	lines := bytes.Split(queryFile, []byte("\n"))

	var extractedLines [][]byte

	for _, line := range lines {
		// Remove any comments, if at the end of the line
		commentIndex := bytes.Index(line, []byte(`--`))
		if commentIndex != -1 {
			// remove everything after comment
			line = line[0:commentIndex]
		}

		if len(line) == 0 {
			continue
		}

		extractedLines = append(extractedLines, bytes.TrimSpace(line))
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
		return nil, errors.New(`invalid template`)
	}

	return queries, nil
}

func getCassandraQueriesFromQueryStrings(session cassandra.Session, queries []string) []cassandra.Query {
	var casQueries []cassandra.Query

	for _, query := range queries {
		casQueries = append(casQueries, session.Query(query))
	}

	return casQueries
}

func contructSchemaQueries(session cassandra.Session, cfg *Schema) ([]cassandra.Query, error) {
	params, err := constructTemplateParams(*cfg)
	if err != nil {
		return nil, err
	}

	queryFile, err := getQueryFileAsBytes(`v004-go-tmpl.cql.tmpl`, params)
	if err != nil {
		return nil, err
	}

	queryStrings, err := getQueriesFromBytes(queryFile)
	if err != nil {
		return nil, err
	}

	casQueries := getCassandraQueriesFromQueryStrings(session, queryStrings)

	return casQueries, nil
}

func generateSchemaIfNotPresent(session cassandra.Session, cfg *Schema) error {
	casQueries, err := contructSchemaQueries(session, cfg)
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
