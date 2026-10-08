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

	v, err = d.Next(1)
	require.NoError(t, err)
	assert.Equal(t, uint(2), v)
	r, identifier, err = d.ReadUp(v)
	require.NoError(t, err)
	defer r.Close()
	assert.Equal(t, "otlp_span_fields", identifier)
	content, err = io.ReadAll(r)
	require.NoError(t, err)
	assert.Contains(t, string(content), "ALTER TABLE spans")
}
