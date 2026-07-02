// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBackendVersion_String(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected string
	}{
		{ElasticV6, "Elasticsearch 6.x"},
		{ElasticV7, "Elasticsearch 7.x"},
		{ElasticV8, "Elasticsearch 8.x"},
		{ElasticV9, "Elasticsearch 9.x"},
		{OpenSearch1, "OpenSearch 1.x"},
		{OpenSearch2, "OpenSearch 2.x"},
		{OpenSearch3, "OpenSearch 3.x"},
		{BackendVersion(999), "Unknown(999)"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.String())
	}
}

func TestBackendVersion_IsOpenSearch(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected bool
	}{
		{ElasticV6, false},
		{ElasticV7, false},
		{ElasticV8, false},
		{ElasticV9, false},
		{OpenSearch1, true},
		{OpenSearch2, true},
		{OpenSearch3, true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.IsOpenSearch(), tt.version.String())
	}
}

func TestBackendVersion_TemplateVersion(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected uint
	}{
		{ElasticV6, 6},
		{ElasticV7, 7},
		{ElasticV8, 8},
		{ElasticV9, 8},
		{OpenSearch1, 7},
		{OpenSearch2, 7},
		{OpenSearch3, 7},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.TemplateVersion(), tt.version.String())
	}
}

func TestBackendVersion_UsesV8API(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected bool
	}{
		{ElasticV6, false},
		{ElasticV7, false},
		{ElasticV8, true},
		{ElasticV9, true},
		{OpenSearch1, false},
		{OpenSearch2, false},
		{OpenSearch3, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.UsesV8API(), tt.version.String())
	}
}

func TestBackendVersion_SupportsComposableTemplates(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected bool
	}{
		{ElasticV6, false},
		{ElasticV7, true},
		{ElasticV8, true},
		{ElasticV9, true},
		{OpenSearch1, false},
		{OpenSearch2, true},
		{OpenSearch3, true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.SupportsComposableTemplates(), tt.version.String())
	}
}

func TestBackendVersion_SupportsTypedIndices(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected bool
	}{
		{ElasticV6, true},
		{ElasticV7, false},
		{ElasticV8, false},
		{ElasticV9, false},
		{OpenSearch1, false},
		{OpenSearch2, false},
		{OpenSearch3, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.SupportsTypedIndices(), tt.version.String())
	}
}

func TestBackendVersion_SupportsILM(t *testing.T) {
	tests := []struct {
		version  BackendVersion
		expected bool
	}{
		{ElasticV6, false},
		{ElasticV7, true},
		{ElasticV8, true},
		{ElasticV9, true},
		{OpenSearch1, true},
		{OpenSearch2, true},
		{OpenSearch3, true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.version.SupportsILM(), tt.version.String())
	}
}

func TestDetectBackendVersion(t *testing.T) {
	tests := []struct {
		tagLine      string
		majorVersion int
		expected     BackendVersion
	}{
		{"You Know, for Search", 6, ElasticV6},
		{"You Know, for Search", 7, ElasticV7},
		{"You Know, for Search", 8, ElasticV8},
		{"You Know, for Search", 9, ElasticV9},
		{"You Know, for Search", 5, ElasticV8},
		{"The OpenSearch Project: https://opensearch.org/", 1, OpenSearch1},
		{"The OpenSearch Project: https://opensearch.org/", 2, OpenSearch2},
		{"The OpenSearch Project: https://opensearch.org/", 3, OpenSearch3},
		{"The OpenSearch Project: https://opensearch.org/", 4, OpenSearch3},
	}
	for _, tt := range tests {
		result := DetectBackendVersion(tt.tagLine, tt.majorVersion)
		assert.Equal(t, tt.expected, result, "tagLine=%q major=%d", tt.tagLine, tt.majorVersion)
	}
}
