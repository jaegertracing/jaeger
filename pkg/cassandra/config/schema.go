// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"text/template"

	"github.com/jaegertracing/jaeger/pkg/cassandra"
)

//go:embed v004-go-tmpl.cql.tmpl
var schemaFile embed.FS

type TemplateParams struct {
	Schema
	// Replication is the replication strategy used
	Replication string `mapstructure:"replication" valid:"optional"`
	// CompactionWindowSize is the numberical part of CompactionWindow. Extracted from CompactionWindow
	CompactionWindowSize int `mapstructure:"compaction_window_size" valid:"optional"`
	// CompactionWindowUnit is the time unit of the CompactionWindow. Extracted from CompactionWindow
	CompactionWindowUnit string `mapstructure:"compaction_window_unit" valid:"optional"`
}

func DefaultParams() TemplateParams {
	return TemplateParams{
		Schema: Schema{
			Keyspace:          "jaeger_v1_dc1",
			Datacenter:        "test",
			TraceTTL:          172800,
			DependenciesTTL:   0,
			ReplicationFactor: 1,
			CasVersion:        4,
		},
	}
}

func applyDefaults(params *TemplateParams) {
	defaultSchema := DefaultParams()

	if params.Keyspace == "" {
		params.Keyspace = defaultSchema.Keyspace
	}

	if params.Datacenter == "" {
		params.Datacenter = defaultSchema.Datacenter
	}

	if params.TraceTTL == 0 {
		params.TraceTTL = defaultSchema.TraceTTL
	}

	if params.ReplicationFactor == 0 {
		params.ReplicationFactor = defaultSchema.ReplicationFactor
	}

	if params.CasVersion == 0 {
		params.CasVersion = 4
	}
}

// Applies defaults for the configs and contructs other optional parameters from it
func constructTemplateParams(cfg Schema) (TemplateParams, error) {
	params := TemplateParams{
		Schema: cfg,
	}
	applyDefaults(&params)

	params.Replication = fmt.Sprintf("{'class': 'NetworkTopologyStrategy', 'replication_factor': '%v' }", params.ReplicationFactor)

	if params.CompactionWindow != "" {
		var err error
		isMatch, err := regexp.MatchString("[0-9]+[mhd]", params.CompactionWindow)
		if err != nil {
			return TemplateParams{}, err
		}

		if !isMatch {
			return TemplateParams{}, fmt.Errorf("invalid compaction window size format: %s. Please use numeric value followed by 'm' for minutes, 'h' for hours, or 'd' for days", cfg.CompactionWindow)
		}

		params.CompactionWindowSize, err = strconv.Atoi(params.CompactionWindow[0 : len(params.CompactionWindow)-1])
		if err != nil {
			return TemplateParams{}, err
		}

		params.CompactionWindowUnit = params.CompactionWindow[len(params.CompactionWindow)-1 : len(params.CompactionWindow)]
	} else {
		traceTTLMinutes := cfg.TraceTTL / 60

		params.CompactionWindowSize = (traceTTLMinutes + 30 - 1) / 30
		params.CompactionWindowUnit = "m"
	}

	switch params.CompactionWindowUnit {
	case `m`:
		params.CompactionWindowUnit = `MINUTES`
	case `h`:
		params.CompactionWindowUnit = `HOURS`
	case `d`:
		params.CompactionWindowUnit = `DAYS`
	default:
		return TemplateParams{}, errors.New("Invalid compaction window unit. If can be among {m|h|d}")
	}

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
		return nil, errors.New(`Invalid template`)
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

func GenerateSchemaIfNotPresent(session cassandra.Session, cfg *Schema) error {
	casQueries, err := contructSchemaQueries(session, cfg)
	if err != nil {
		return err
	}

	for _, query := range casQueries {
		err := query.Exec()
		if err != nil {
			return err
		}
	}

	return nil
}
