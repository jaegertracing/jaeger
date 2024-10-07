// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// func TestRenderTemplate(t *testing.T) {
// 	mb := &MappingBuilder{
// 		Mode:                 "test",
// 		Datacentre:           "dc1",
// 		Keyspace:             "test_keyspace",
// 		Replication:          "{'class': 'SimpleStrategy', 'replication_factor': '1'}",
// 		TraceTTL:             172800,
// 		DependenciesTTL:      0,
// 		CompactionWindowSize: 30,
// 		CompactionWindowUnit: "MINUTES",
// 	}

// 	// Create a temporary template file
// 	templateContent := "CREATE KEYSPACE ${keyspace} WITH replication = ${replication};"
// 	tmpFile, err := os.CreateTemp("", "template.cql.tmpl")
// 	assert.NoError(t, err)
// 	defer os.Remove(tmpFile.Name())

// 	_, err = tmpFile.WriteString(templateContent)
// 	assert.NoError(t, err)
// 	tmpFile.Close()

// 	renderedOutput, err := mb.renderTemplate(tmpFile.Name())
// 	assert.NoError(t, err)
// 	expectedOutput := "CREATE KEYSPACE test_keyspace WITH replication = {'class': 'SimpleStrategy', 'replication_factor': '1'};"
// 	assert.Equal(t, expectedOutput, renderedOutput)
// }

func TestRenderCQLTemplate(t *testing.T) {
	params := MappingBuilder{
		Keyspace:             "test_keyspace",
		Replication:          "{'class': 'SimpleStrategy', 'replication_factor': '1'}",
		TraceTTL:             172800,
		DependenciesTTL:      0,
		CompactionWindowSize: 30,
		CompactionWindowUnit: "MINUTES",
	}

	cqlOutput := `
		CREATE KEYSPACE ${keyspace} WITH replication = ${replication};
		CREATE TABLE ${keyspace}.traces (trace_id UUID PRIMARY KEY, span_id UUID, trace_ttl int);
	`

	renderedOutput, err := RenderCQLTemplate(params, cqlOutput)
	assert.NoError(t, err)
	expectedOutput := `
		CREATE KEYSPACE test_keyspace WITH replication = {'class': 'SimpleStrategy', 'replication_factor': '1'};
		CREATE TABLE test_keyspace.traces (trace_id UUID PRIMARY KEY, span_id UUID, trace_ttl int);
	`
	assert.Equal(t, strings.TrimSpace(expectedOutput), strings.TrimSpace(renderedOutput))
}

func TestGetCompactionWindow(t *testing.T) {
	tests := []struct {
		traceTTL         int
		compactionWindow string
		expectedSize     int
		expectedUnit     string
		expectError      bool
	}{
		{172800, "30m", 30, "MINUTES", false},
		{172800, "2h", 2, "HOURS", false},
		{172800, "1d", 1, "DAYS", false},
		{172800, "", 96, "MINUTES", false},
		{172800, "invalid", 0, "", true},
	}

	for _, test := range tests {
		size, unit, err := getCompactionWindow(test.traceTTL, test.compactionWindow)
		if test.expectError {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, test.expectedSize, size)
			assert.Equal(t, test.expectedUnit, unit)
		}
	}
}

func TestIsValidKeyspace(t *testing.T) {
	assert.True(t, isValidKeyspace("valid_keyspace"))
	assert.False(t, isValidKeyspace("invalid-keyspace"))
	assert.False(t, isValidKeyspace("invalid keyspace"))
}

func TestGetSpanServiceMappings(t *testing.T) {
	os.Setenv("TRACE_TTL", "172800")
	os.Setenv("DEPENDENCIES_TTL", "0")
	os.Setenv("VERSION", "4")
	os.Setenv("MODE", "test")
	os.Setenv("DATACENTRE", "dc1")
	os.Setenv("REPLICATION_FACTOR", "1")
	os.Setenv("KEYSPACE", "test_keyspace")

	builder := &MappingBuilder{}
	spanMapping, serviceMapping, err := builder.GetSpanServiceMappings()
	assert.NoError(t, err)
	assert.NotEmpty(t, spanMapping)
	assert.Empty(t, serviceMapping)
}
