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
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestConstructSchemaQueries(t *testing.T) {
	queries, err := constructSchemaQueries()
	if err != nil {
		t.Fatal(err)
	}

	assert.Contains(t, queries[0], "CREATE DATABASE IF NOT EXISTS jaeger;")
	assert.Contains(t, queries[1], "CREATE TABLE IF NOT EXISTS jaeger.otel_traces")
}

func TestCreateSchemaIfNotPresent(t *testing.T) {
	t.Run("should succeed when schema is created successfully", func(t *testing.T) {
		pool := mocks.Pool{}
		pool.On("Do", mock.Anything, mock.Anything).Return(nil)
		err := CreateSchemaIfNotPresent(&pool)
		assert.NoError(t, err)
	})

	t.Run("should fail when unable to connect to ClickHouse server", func(t *testing.T) {
		pool := mocks.Pool{}
		pool.On("Do", mock.Anything, mock.Anything).Return(errors.New("can't connect to clickhouse server"))
		err := CreateSchemaIfNotPresent(&pool)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "can't connect to clickhouse server")
	})
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
