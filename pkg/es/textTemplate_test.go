// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package es

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	const wantString = "text/template parse"
	values := struct {
		Str string
	}{wantString}

	template := "Parse is a wrapper for {{ .Str }} function."
	writer := new(bytes.Buffer)
	testTemplateBuilder := TextTemplateBuilder{}
	parsedStr, err := testTemplateBuilder.Parse(template)
	require.NoError(t, err)
	err = parsedStr.Execute(writer, values)
	require.NoError(t, err)
	assert.Equal(t, "Parse is a wrapper for text/template parse function.", writer.String())
}
