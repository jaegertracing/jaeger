// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestGetQueryFileAsBytes(t *testing.T) {
	t.Run("should return error when file not exist", func(t *testing.T) {
		bts, err := getQueryFileAsBytes("not_exist")
		require.Error(t, err)
		assert.Nil(t, bts)
	})
}

func TestGetQueriesFromBytes(t *testing.T) {
	t.Run("should successful when query segment lost ;", func(t *testing.T) {
		queryFile := []byte(`
		SELECT * FROM otel_traces;
		SELECT * FROM xxx
		`)
		queries, err := getQueriesFromBytes(queryFile)
		require.NoError(t, err)
		require.Equal(t, "SELECT * FROM otel_traces; ", queries[0])
		require.Equal(t, "SELECT * FROM xxx ", queries[1])
	})
}

func TestConstructSchemaQueries(t *testing.T) {
	t.Run("should successful when everything fine", func(t *testing.T) {
		queries, err := constructSchemaQueries(schema)
		if err != nil {
			t.Fatal(err)
		}

		assert.Contains(t, queries[0], "CREATE TABLE IF NOT EXISTS otel_traces")
	})

	t.Run("should return error when schema file not found.", func(t *testing.T) {
		queries, err := constructSchemaQueries("not found")
		require.Error(t, err)
		assert.Nil(t, queries)
	})
}

func TestCreateSchemaIfNotPresent(t *testing.T) {
	t.Run("should successful when schema is created successfully", func(t *testing.T) {
		pool := mocks.ChPool{}
		pool.On("Do", mock.Anything, mock.Anything).Return(nil)
		err := CreateSchemaIfNotPresent(&pool)
		assert.NoError(t, err)
	})

	t.Run("should fail when unable to connect to ClickHouse server", func(t *testing.T) {
		pool := mocks.ChPool{}
		pool.On("Do", mock.Anything, mock.Anything).Return(errors.New("can't connect to clickhouse server"))
		err := CreateSchemaIfNotPresent(&pool)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "can't connect to clickhouse server")
	})
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
