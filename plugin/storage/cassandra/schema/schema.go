// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"embed"
	"fmt"
	"regexp"
	"strconv"
	"text/template"

	"github.com/jaegertracing/jaeger/pkg/cassandra"
	"github.com/jaegertracing/jaeger/pkg/cassandra/config"
)

//go:embed v004-go-tmpl.cql.tmpl
//go:embed v004-go-tmpl-test.cql.tmpl
var schemaFile embed.FS

func DefaultSchema() config.Schema {
	return config.Schema{
		Keyspace:          "jaeger_v2_test",
		Datacenter:        "test",
		TraceTTL:          172800,
		DependenciesTTL:   0,
		ReplicationFactor: 1,
		CasVersion:        4,
	}
}

func applyDefaults(cfg *config.Schema) {
	defaultSchema := DefaultSchema()

	if cfg.Keyspace == "" {
		cfg.Keyspace = defaultSchema.Keyspace
	}

	if cfg.Datacenter == "" {
		cfg.Datacenter = defaultSchema.Datacenter
	}

	if cfg.TraceTTL == 0 {
		cfg.TraceTTL = defaultSchema.TraceTTL
	}

	if cfg.ReplicationFactor == 0 {
		cfg.ReplicationFactor = defaultSchema.ReplicationFactor
	}

	if cfg.CasVersion == 0 {
		cfg.CasVersion = 4
	}
}

// Applies defaults for the configs and contructs other optional parameters from it
func constructCompleteSchema(cfg *config.Schema) error {
	applyDefaults(cfg)

	cfg.Replication = fmt.Sprintf("{'class': 'NetworkTopologyStrategy', '%s': '%v' }", cfg.Datacenter, cfg.ReplicationFactor)

	if cfg.CompactionWindow != "" {
		isMatch, err := regexp.MatchString("^[0-9]+[mhd]$", cfg.CompactionWindow)
		if err != nil {
			return err
		}

		if !isMatch {
			return fmt.Errorf("Invalid compaction window size format. Please use numeric value followed by 'm' for minutes, 'h' for hours, or 'd' for days")
		}

		cfg.CompactionWindowSize, err = strconv.Atoi(cfg.CompactionWindow[:len(cfg.CompactionWindow)-1])
		if err != nil {
			return err
		}

		cfg.CompactionWindowUnit = cfg.CompactionWindow[len(cfg.CompactionWindow)-1:]
	} else {
		traceTTLMinutes := cfg.TraceTTL / 60

		cfg.CompactionWindowSize = (traceTTLMinutes + 30 - 1) / 30
		cfg.CompactionWindowUnit = "m"
	}

	switch cfg.CompactionWindowUnit {
	case `m`:
		cfg.CompactionWindowUnit = `MINUTES`
	case `h`:
		cfg.CompactionWindowUnit = `HOURS`
	case `d`:
		cfg.CompactionWindowUnit = `DAYS`
	default:
		return fmt.Errorf("Invalid compaction window unit. If can be among {m|h|d}")
	}

	return nil
}

func getQueryFileAsBytes(fileName string, cfg *config.Schema) ([]byte, error) {
	tmpl, err := template.ParseFS(schemaFile, fileName)
	if err != nil {
		return nil, err
	}

	var result bytes.Buffer
	err = tmpl.Execute(&result, cfg)
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
		return nil, fmt.Errorf(`Invalid template`)
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

func contructSchemaQueries(session cassandra.Session, cfg *config.Schema) ([]cassandra.Query, error) {
	err := constructCompleteSchema(cfg)
	if err != nil {
		return nil, err
	}

	queryFile, err := getQueryFileAsBytes(`v004-go-tmpl.cql.tmpl`, cfg)
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

func GenerateSchemaIfNotPresent(session cassandra.Session, cfg *config.Schema) error {
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
