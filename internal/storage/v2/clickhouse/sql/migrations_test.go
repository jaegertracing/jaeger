// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"io"
	"testing"

	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationFiles(t *testing.T) {
	d, err := iofs.New(MigrationFiles, ".")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, d.Close())
	}()

	v, err := d.First()
	require.NoError(t, err)
	assert.Equal(t, uint(1), v)

	r, identifier, err := d.ReadUp(1)
	require.NoError(t, err)
	defer r.Close()

	assert.Equal(t, "initial_schema", identifier)
	content, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Contains(t, string(content), "CREATE TABLE\n    IF NOT EXISTS spans")

	v2, err := d.Next(1)
	require.NoError(t, err)
	assert.Equal(t, uint(2), v2)

	r2, identifier2, err := d.ReadUp(2)
	require.NoError(t, err)
	defer r2.Close()

	assert.Equal(t, "otlp_span_fields", identifier2)
	content2, err := io.ReadAll(r2)
	require.NoError(t, err)
	assert.Contains(t, string(content2), "ALTER TABLE spans ADD COLUMN IF NOT EXISTS resource_schema_url String")
}
