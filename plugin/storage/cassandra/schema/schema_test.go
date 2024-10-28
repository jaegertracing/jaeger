// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateRendering(t *testing.T) {
	cfg := DefaultSchemaConfig()
	res, err := getQueryFileAsBytes(`v004-go-tmpl-test.cql.tmpl`, &cfg)
	require.NoError(t, err)

	queryStrings, err := getQueriesFromBytes(res)
	require.NoError(t, err)

	assert.Equal(t, 9, len(queryStrings))
}
