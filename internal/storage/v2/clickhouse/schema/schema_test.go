// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
