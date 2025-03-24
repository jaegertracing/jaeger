// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"context"
	"embed"
	"strings"
	"text/template"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
)

//go:embed schema.tmpl
var schemaFile embed.FS

const schema = "schema.tmpl"

func getQueryFileAsBytes(fileName string) ([]byte, error) {
	tmpl, err := template.ParseFS(schemaFile, fileName)
	if err != nil {
		return nil, err
	}

	var result bytes.Buffer
	err = tmpl.Execute(&result, nil)
	if err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func getQueriesFromBytes(queryFile []byte) ([]string, error) {
	lines := bytes.Split(queryFile, []byte("\n"))

	var queryStrings []string
	var one strings.Builder

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(string(line))

		if trimmedLine == "" {
			continue
		}

		one.WriteString(trimmedLine)
		one.WriteString(" ")
		if strings.HasSuffix(trimmedLine, ";") {
			queryStrings = append(queryStrings, one.String())
			one.Reset()
		}
	}

	if one.Len() > 0 {
		queryStrings = append(queryStrings, one.String())
	}

	return queryStrings, nil
}

func constructSchemaQueries(schema string) ([]string, error) {
	queryFile, err := getQueryFileAsBytes(schema)
	if err != nil {
		return nil, err
	}
	queryStrings, err := getQueriesFromBytes(queryFile)
	if err != nil {
		return nil, err
	}
	return queryStrings, nil
}

func CreateSchemaIfNotPresent(pool client.ChPool) error {
	queries, err := constructSchemaQueries(schema)
	if err != nil {
		return err
	}

	for _, query := range queries {
		err := pool.Do(context.Background(), query)
		if err != nil {
			return err
		}
	}
	return nil
}
