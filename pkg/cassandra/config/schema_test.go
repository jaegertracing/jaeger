// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryGenerationFromBytes(t *testing.T) {
	queriesAsString := `
query1 -- comment (this should be removed)
query1-continue
query1-finished; --


query2;
query-3 query-3-continue query-3-finished;
`
	expGeneratedQueries := []string{
		`query1
query1-continue
query1-finished;
`,
		`query2;
`,
		`query-3 query-3-continue query-3-finished;
`,
	}

	sc := schemaCreator{}
	queriesAsBytes := []byte(queriesAsString)
	queries, err := sc.getQueriesFromBytes(queriesAsBytes)
	require.NoError(t, err)

	require.Equal(t, len(expGeneratedQueries), len(queries))

	for i := range len(expGeneratedQueries) {
		require.Equal(t, expGeneratedQueries[i], queries[i])
	}
}

func TestInvalidQueryTemplate(t *testing.T) {
	queriesAsString := `
	query1 -- comment (this should be removed)
	query1-continue
	query1-finished; --


	query2;
	query-3 query-3-continue query-3-finished -- missing semicolon
	`
	sc := schemaCreator{}
	queriesAsBytes := []byte(queriesAsString)
	_, err := sc.getQueriesFromBytes(queriesAsBytes)
	require.Error(t, err)
}
